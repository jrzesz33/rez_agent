"""
Agent Response Handler
Handles polling and processing of tool execution results from the agent response queue.
"""
import json
import logging
from datetime import datetime
from typing import Dict, Any, List, Optional

import boto3
from botocore.exceptions import ClientError

logger = logging.getLogger()


class ResponseHandler:
    """Handles receiving and processing tool responses from SQS"""

    def __init__(self, queue_url: str, max_messages: int = 10, wait_time: int = 5):
        """
        Initialize response handler.

        Args:
            queue_url: SQS queue URL for agent responses
            max_messages: Maximum messages to receive per poll (1-10)
            wait_time: Long polling wait time in seconds (0-20)
        """
        self.sqs_client = boto3.client("sqs")
        self.queue_url = queue_url
        self.max_messages = min(max_messages, 10)
        self.wait_time = min(wait_time, 20)

    def poll_responses(self, timeout_seconds: int = 30) -> List[Dict[str, Any]]:
        """
        Poll the response queue for tool execution results.

        Args:
            timeout_seconds: Maximum time to poll for responses

        Returns:
            List of parsed response messages
        """
        responses = []
        start_time = datetime.utcnow()

        try:
            while (datetime.utcnow() - start_time).total_seconds() < timeout_seconds:
                # Long poll for messages
                response = self.sqs_client.receive_message(
                    QueueUrl=self.queue_url,
                    MaxNumberOfMessages=self.max_messages,
                    WaitTimeSeconds=self.wait_time,
                    MessageAttributeNames=["All"],
                    AttributeNames=["All"],
                )

                messages = response.get("Messages", [])
                if not messages:
                    # No messages available
                    break

                logger.info(f"Received {len(messages)} response messages from queue")

                for message in messages:
                    try:
                        # Parse message body
                        body = json.loads(message["Body"])

                        # Extract tool response data
                        parsed_response = {
                            "message_id": body.get("id"),
                            "created_by": body.get("created_by"),
                            "created_date": body.get("created_date"),
                            "message_type": body.get("message_type"),
                            "status": body.get("status"),
                            "payload": body.get("payload"),
                            "stage": body.get("stage"),
                            "receipt_handle": message["ReceiptHandle"],
                        }

                        responses.append(parsed_response)

                        # Delete message from queue
                        self._delete_message(message["ReceiptHandle"])

                    except Exception as e:
                        logger.error(f"Error parsing response message: {e}", exc_info=True)
                        # Don't delete - let it retry or go to DLQ
                        continue

                # Break if we got some responses
                if responses:
                    break

        except ClientError as e:
            logger.error(f"Error polling response queue: {e}", exc_info=True)

        return responses

    def poll_for_message(self, original_message_id: str, timeout_seconds: int = 30) -> Optional[Dict[str, Any]]:
        """
        Poll for a specific message response.

        Args:
            original_message_id: The ID of the original request message
            timeout_seconds: Maximum time to wait for response

        Returns:
            Parsed response message or None if not found
        """
        start_time = datetime.utcnow()

        while (datetime.utcnow() - start_time).total_seconds() < timeout_seconds:
            responses = self.poll_responses(timeout_seconds=5)

            for response in responses:
                # Check if this is a response to our message
                # This is a simplified check - you might want to add correlation IDs
                if response.get("created_date"):
                    logger.info(f"Found response: {response['message_id']}")
                    return response

            if responses:
                # Got responses but not for our message
                # These have been deleted, continue polling
                continue

        logger.warning(f"No response found for message {original_message_id} within {timeout_seconds}s")
        return None

    def _delete_message(self, receipt_handle: str) -> None:
        """Delete a message from the queue"""
        try:
            self.sqs_client.delete_message(
                QueueUrl=self.queue_url,
                ReceiptHandle=receipt_handle,
            )
            logger.debug(f"Deleted message from queue")
        except ClientError as e:
            logger.error(f"Error deleting message: {e}")

    def format_response_for_agent(self, response: Dict[str, Any]) -> str:
        """
        Format a tool response for inclusion in agent context.

        Args:
            response: Parsed response message

        Returns:
            Formatted string for agent consumption
        """
        payload = response.get("payload", "")
        message_type = response.get("message_type", "unknown")
        status = response.get("status", "unknown")

        formatted = f"Tool Response (status: {status}):\n{payload}"

        return formatted

    def get_queue_depth(self) -> int:
        """Get approximate number of messages in queue"""
        try:
            response = self.sqs_client.get_queue_attributes(
                QueueUrl=self.queue_url,
                AttributeNames=["ApproximateNumberOfMessages"],
            )
            return int(response["Attributes"]["ApproximateNumberOfMessages"])
        except ClientError as e:
            logger.error(f"Error getting queue depth: {e}")
            return 0
