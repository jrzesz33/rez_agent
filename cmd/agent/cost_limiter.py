"""
Bedrock Cost Limiter
Implements a daily spending cap for Bedrock API calls to prevent cost overruns.
"""
import json
import logging
from datetime import datetime, timezone
from decimal import Decimal
from typing import Optional, Tuple

import boto3
from botocore.exceptions import ClientError

logger = logging.getLogger()

# Pricing for Claude 3.5 Sonnet v2 (as of Jan 2025)
# Source: https://aws.amazon.com/bedrock/pricing/
CLAUDE_3_5_SONNET_PRICING = {
    "input_per_1k_tokens": Decimal("0.003"),    # $3 per million input tokens
    "output_per_1k_tokens": Decimal("0.015"),   # $15 per million output tokens
}

DAILY_SPENDING_CAP = Decimal("5.00")  # $5 per day


class CostLimiter:
    """Manages daily spending limits for Bedrock API calls"""

    def __init__(self, dynamodb_table_name: str, stage: str):
        self.dynamodb = boto3.resource("dynamodb")
        self.table = self.dynamodb.Table(dynamodb_table_name)
        self.stage = stage
        self.cost_tracking_key = f"bedrock_cost_tracker_{stage}"

    def _get_current_date(self) -> str:
        """Get current UTC date in YYYY-MM-DD format"""
        return datetime.now(timezone.utc).strftime("%Y-%m-%d")

    def _get_cost_record(self) -> dict:
        """Get today's cost tracking record from DynamoDB"""
        try:
            response = self.table.get_item(
                Key={"id": self.cost_tracking_key}
            )

            if "Item" in response:
                record = response["Item"]

                # Check if record is from today
                current_date = self._get_current_date()
                if record.get("date") == current_date:
                    return record
                else:
                    # Record is from a previous day, reset it
                    logger.info(f"Cost record from {record.get('date')}, resetting for {current_date}")
                    return self._initialize_cost_record()
            else:
                # No record exists, create new one
                return self._initialize_cost_record()

        except ClientError as e:
            logger.error(f"Error getting cost record: {e}")
            # On error, be conservative and assume limit reached
            return {
                "id": self.cost_tracking_key,
                "date": self._get_current_date(),
                "total_cost": str(DAILY_SPENDING_CAP),
                "request_count": 0,
                "input_tokens": 0,
                "output_tokens": 0,
            }

    def _initialize_cost_record(self) -> dict:
        """Initialize a new cost tracking record"""
        record = {
            "id": self.cost_tracking_key,
            "date": self._get_current_date(),
            "total_cost": "0.00",
            "request_count": 0,
            "input_tokens": 0,
            "output_tokens": 0,
            "last_updated": datetime.now(timezone.utc).isoformat(),
        }

        try:
            self.table.put_item(Item=record)
        except ClientError as e:
            logger.error(f"Error initializing cost record: {e}")

        return record

    def _save_cost_record(self, record: dict) -> None:
        """Save cost tracking record to DynamoDB"""
        try:
            record["last_updated"] = datetime.now(timezone.utc).isoformat()
            self.table.put_item(Item=record)
        except ClientError as e:
            logger.error(f"Error saving cost record: {e}")

    def calculate_cost(self, input_tokens: int, output_tokens: int) -> Decimal:
        """Calculate cost for given token usage"""
        input_cost = (Decimal(input_tokens) / 1000) * CLAUDE_3_5_SONNET_PRICING["input_per_1k_tokens"]
        output_cost = (Decimal(output_tokens) / 1000) * CLAUDE_3_5_SONNET_PRICING["output_per_1k_tokens"]
        return input_cost + output_cost

    def check_and_update_cost(
        self,
        estimated_input_tokens: int = 4000,  # Conservative estimate
        estimated_output_tokens: int = 2000   # Conservative estimate
    ) -> Tuple[bool, str, dict]:
        """
        Check if request would exceed daily spending cap and update cost if allowed.

        Args:
            estimated_input_tokens: Estimated input tokens for the request
            estimated_output_tokens: Estimated output tokens for the request

        Returns:
            Tuple of (allowed: bool, message: str, cost_info: dict)
        """
        # Get current cost record
        record = self._get_cost_record()

        current_cost = Decimal(record["total_cost"])
        estimated_cost = self.calculate_cost(estimated_input_tokens, estimated_output_tokens)
        projected_cost = current_cost + estimated_cost

        cost_info = {
            "current_cost": float(current_cost),
            "estimated_cost": float(estimated_cost),
            "projected_cost": float(projected_cost),
            "daily_cap": float(DAILY_SPENDING_CAP),
            "remaining_budget": float(DAILY_SPENDING_CAP - current_cost),
            "request_count": record["request_count"],
            "reset_time": f"{self._get_current_date()} 23:59:59 UTC",
        }

        # Check if projected cost would exceed cap
        if projected_cost > DAILY_SPENDING_CAP:
            message = (
                f"Daily spending cap of ${DAILY_SPENDING_CAP} would be exceeded. "
                f"Current usage: ${current_cost:.2f}, "
                f"Estimated request cost: ${estimated_cost:.2f}. "
                f"Resets at midnight UTC ({cost_info['reset_time']})."
            )
            logger.warning(message)
            return False, message, cost_info

        # Update cost record (optimistically - actual cost will be updated after LLM call)
        record["total_cost"] = str(projected_cost)
        record["request_count"] = record["request_count"] + 1
        record["input_tokens"] = record.get("input_tokens", 0) + estimated_input_tokens
        record["output_tokens"] = record.get("output_tokens", 0) + estimated_output_tokens
        self._save_cost_record(record)

        message = f"Request allowed. Projected cost: ${projected_cost:.2f} / ${DAILY_SPENDING_CAP}"
        logger.info(message)
        return True, message, cost_info

    def update_actual_cost(self, input_tokens: int, output_tokens: int) -> None:
        """
        Update cost record with actual token usage after LLM call completes.
        This corrects the optimistic estimate made in check_and_update_cost.
        """
        record = self._get_cost_record()

        # Recalculate total cost based on actual tokens
        actual_cost = self.calculate_cost(input_tokens, output_tokens)

        # Update record
        record["input_tokens"] = record.get("input_tokens", 0) + input_tokens
        record["output_tokens"] = record.get("output_tokens", 0) + output_tokens

        # Recalculate total from scratch to avoid accumulation errors
        total_input_tokens = record["input_tokens"]
        total_output_tokens = record["output_tokens"]
        record["total_cost"] = str(self.calculate_cost(total_input_tokens, total_output_tokens))

        self._save_cost_record(record)

        logger.info(
            f"Updated actual cost: ${record['total_cost']} "
            f"(input: {input_tokens}, output: {output_tokens})"
        )

    def get_current_usage(self) -> dict:
        """Get current usage statistics"""
        record = self._get_cost_record()

        current_cost = Decimal(record["total_cost"])
        remaining = DAILY_SPENDING_CAP - current_cost

        return {
            "date": record["date"],
            "total_cost": float(current_cost),
            "daily_cap": float(DAILY_SPENDING_CAP),
            "remaining_budget": float(remaining),
            "percentage_used": float((current_cost / DAILY_SPENDING_CAP) * 100),
            "request_count": record["request_count"],
            "input_tokens": record.get("input_tokens", 0),
            "output_tokens": record.get("output_tokens", 0),
            "reset_time": f"{record['date']} 23:59:59 UTC",
        }
