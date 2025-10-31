"""
Agent Tools for Golf Reservation System
Tools that the AI agent can use to interact with the golf system.
"""
import json
import logging
import os
import uuid
from datetime import datetime
from typing import Any, Dict, Optional

import boto3
from langchain_core.tools import tool

# Configure logging
logger = logging.getLogger()

# Environment variables
STAGE = os.environ.get("STAGE", "dev")
WEB_ACTIONS_TOPIC_ARN = os.environ.get("WEB_ACTIONS_TOPIC_ARN")
NOTIFICATIONS_TOPIC_ARN = os.environ.get("NOTIFICATIONS_TOPIC_ARN")
DYNAMODB_TABLE_NAME = os.environ.get("DYNAMODB_TABLE_NAME")

# AWS clients
sns_client = boto3.client("sns")
dynamodb = boto3.resource("dynamodb")


def publish_to_web_actions(message_data: Dict[str, Any]) -> str:
    """Publish a message to the web actions topic"""
    try:
        # Create message with metadata
        message = {
            "id": f"msg_{datetime.utcnow().strftime('%Y%m%d%H%M%S')}_{uuid.uuid4().hex[:6]}",
            "created_date": datetime.utcnow().isoformat() + "Z",
            "created_by": "ai-agent",
            "stage": STAGE,
            "message_type": "web_action",
            "status": "created",
            "payload": json.dumps(message_data),
            "retry_count": 0
        }

        # Publish to SNS
        response = sns_client.publish(
            TopicArn=WEB_ACTIONS_TOPIC_ARN,
            Message=json.dumps(message),
            MessageAttributes={
                "stage": {
                    "DataType": "String",
                    "StringValue": STAGE
                },
                "message_type": {
                    "DataType": "String",
                    "StringValue": "web_action"
                },
                "created_by": {
                    "DataType": "String",
                    "StringValue": "ai-agent"
                }
            }
        )

        logger.info(f"Published web action message: {message['id']}")
        return message["id"]

    except Exception as e:
        logger.error(f"Error publishing web action: {e}", exc_info=True)
        raise


@tool
def send_notification_tool(message: str) -> str:
    """
    Send a push notification to the user via ntfy.

    Args:
        message: The notification message to send

    Returns:
        Confirmation message
    """
    try:
        # Create notification message
        notification_data = {
            "version": "1.0",
            "payload": message,
            "stage": STAGE,
            "message_type": "notify"
        }

        # Create message with metadata
        message_obj = {
            "id": f"msg_{datetime.utcnow().strftime('%Y%m%d%H%M%S')}_{uuid.uuid4().hex[:6]}",
            "created_date": datetime.utcnow().isoformat() + "Z",
            "created_by": "ai-agent",
            "stage": STAGE,
            "message_type": "notify",
            "status": "created",
            "payload": message,
            "retry_count": 0
        }

        # Publish to notifications topic
        response = sns_client.publish(
            TopicArn=NOTIFICATIONS_TOPIC_ARN,
            Message=json.dumps(message_obj),
            MessageAttributes={
                "stage": {
                    "DataType": "String",
                    "StringValue": STAGE
                },
                "message_type": {
                    "DataType": "String",
                    "StringValue": "notify"
                }
            }
        )

        logger.info(f"Sent notification: {message_obj['id']}")
        return f"Notification sent successfully: {message}"

    except Exception as e:
        logger.error(f"Error sending notification: {e}", exc_info=True)
        return f"Failed to send notification: {str(e)}"


@tool
def get_reservations_tool(course_name: str) -> str:
    """
    Get a user's upcoming golf reservations for a specific course.

    Args:
        course_name: Name of the golf course (e.g., "Birdsfoot Golf Course" or "Totteridge")

    Returns:
        JSON string with reservation details
    """
    try:
        # Map course name to course info (simplified)
        course_urls = {
            "birdsfoot": "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/UpcomingReservation",
            "totteridge": "https://totteridge.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/UpcomingReservation",
        }

        course_key = course_name.lower()
        if "birdsfoot" in course_key:
            url = course_urls["birdsfoot"]
            token_url = "https://birdsfoot.cps.golf/identityapi/connect/token"
            jwks_url = "https://birdsfoot.cps.golf/identityapi/.well-known/openid-configuration/jwks"
        elif "totteridge" in course_key:
            url = course_urls["totteridge"]
            token_url = "https://totteridge.cps.golf/identityapi/connect/token"
            jwks_url = "https://totteridge.cps.golf/identityapi/.well-known/openid-configuration/jwks"
        else:
            return f"Unknown course: {course_name}. Available courses: Birdsfoot Golf Course, Totteridge"

        # Create web action request
        web_action = {
            "version": "1.0",
            "url": url,
            "action": "golf",
            "arguments": {
                "operation": "fetch_reservations",
                "max_results": 10
            },
            "auth_config": {
                "type": "oauth_password",
                "token_url": token_url,
                "secret_name": f"rez-agent/golf/credentials-prod",
                "jwks_url": jwks_url
            },
            "stage": STAGE,
            "message_type": "web_action"
        }

        # Publish message
        message_id = publish_to_web_actions(web_action)

        return f"Request sent to fetch reservations for {course_name}. Message ID: {message_id}. The results will be available shortly."

    except Exception as e:
        logger.error(f"Error getting reservations: {e}", exc_info=True)
        return f"Failed to get reservations: {str(e)}"


@tool
def search_tee_times_tool(
    course_name: str,
    date: str,
    start_time: str,
    end_time: Optional[str] = None,
    number_of_players: int = 1,
    auto_book: bool = False
) -> str:
    """
    Search for available tee times at a golf course.

    Args:
        course_name: Name of the golf course (e.g., "Birdsfoot Golf Course" or "Totteridge")
        date: Date to search in YYYY-MM-DD format
        start_time: Start time in HH:MM format (24-hour)
        end_time: Optional end time in HH:MM format (24-hour). If not provided, searches until end of day.
        number_of_players: Number of players (1-4), defaults to 1
        auto_book: If True, automatically books the earliest available time

    Returns:
        JSON string with available tee times
    """
    try:
        # Validate inputs
        if number_of_players < 1 or number_of_players > 4:
            return "Number of players must be between 1 and 4"

        # Map course name to course URL
        course_urls = {
            "birdsfoot": "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/TeeTimes",
            "totteridge": "https://totteridge.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/TeeTimes",
        }

        course_key = course_name.lower()
        if "birdsfoot" in course_key:
            url = course_urls["birdsfoot"]
            token_url = "https://birdsfoot.cps.golf/identityapi/connect/token"
            jwks_url = "https://birdsfoot.cps.golf/identityapi/.well-known/openid-configuration/jwks"
        elif "totteridge" in course_key:
            url = course_urls["totteridge"]
            token_url = "https://totteridge.cps.golf/identityapi/connect/token"
            jwks_url = "https://totteridge.cps.golf/identityapi/.well-known/openid-configuration/jwks"
        else:
            return f"Unknown course: {course_name}. Available courses: Birdsfoot Golf Course, Totteridge"

        # Format datetime strings
        start_datetime = f"{date}T{start_time}:00"
        end_datetime = f"{date}T{end_time}:00" if end_time else f"{date}T23:59:00"

        # Create web action request
        web_action = {
            "version": "1.0",
            "url": url,
            "action": "golf",
            "arguments": {
                "operation": "search_tee_times",
                "startSearchTime": start_datetime,
                "endSearchTime": end_datetime,
                "numberOfPlayer": number_of_players,
                "autoBook": auto_book
            },
            "auth_config": {
                "type": "oauth_password",
                "token_url": token_url,
                "secret_name": f"rez-agent/golf/credentials-prod",
                "jwks_url": jwks_url
            },
            "stage": STAGE,
            "message_type": "web_action"
        }

        # Publish message
        message_id = publish_to_web_actions(web_action)

        action_text = "and auto-booking" if auto_book else ""
        return f"Request sent to search {action_text} tee times for {course_name} on {date} from {start_time}{f' to {end_time}' if end_time else ''}. Message ID: {message_id}. The results will be available shortly."

    except Exception as e:
        logger.error(f"Error searching tee times: {e}", exc_info=True)
        return f"Failed to search tee times: {str(e)}"


@tool
def book_tee_time_tool(course_name: str, tee_sheet_id: int) -> str:
    """
    Book a specific tee time at a golf course.

    Args:
        course_name: Name of the golf course (e.g., "Birdsfoot Golf Course" or "Totteridge")
        tee_sheet_id: The tee sheet ID from search results

    Returns:
        Confirmation message
    """
    try:
        # Map course name to course URL
        course_urls = {
            "birdsfoot": "https://birdsfoot.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/ReserveTeeTimes",
            "totteridge": "https://totteridge.cps.golf/onlineres/onlineapi/api/v1/onlinereservation/ReserveTeeTimes",
        }

        course_key = course_name.lower()
        if "birdsfoot" in course_key:
            url = course_urls["birdsfoot"]
            token_url = "https://birdsfoot.cps.golf/identityapi/connect/token"
            jwks_url = "https://birdsfoot.cps.golf/identityapi/.well-known/openid-configuration/jwks"
        elif "totteridge" in course_key:
            url = course_urls["totteridge"]
            token_url = "https://totteridge.cps.golf/identityapi/connect/token"
            jwks_url = "https://totteridge.cps.golf/identityapi/.well-known/openid-configuration/jwks"
        else:
            return f"Unknown course: {course_name}. Available courses: Birdsfoot Golf Course, Totteridge"

        # Create web action request
        web_action = {
            "version": "1.0",
            "url": url,
            "action": "golf",
            "arguments": {
                "operation": "book_tee_time",
                "teeSheetId": tee_sheet_id
            },
            "auth_config": {
                "type": "oauth_password",
                "token_url": token_url,
                "secret_name": f"rez-agent/golf/credentials-prod",
                "jwks_url": jwks_url
            },
            "stage": STAGE,
            "message_type": "web_action"
        }

        # Publish message
        message_id = publish_to_web_actions(web_action)

        return f"Request sent to book tee time (ID: {tee_sheet_id}) at {course_name}. Message ID: {message_id}. You will receive a confirmation shortly."

    except Exception as e:
        logger.error(f"Error booking tee time: {e}", exc_info=True)
        return f"Failed to book tee time: {str(e)}"


@tool
def get_weather_tool(course_name: str, days: int = 2) -> str:
    """
    Get weather forecast for a golf course.

    Args:
        course_name: Name of the golf course (e.g., "Birdsfoot Golf Course" or "Totteridge")
        days: Number of days to forecast (default: 2)

    Returns:
        Weather forecast information
    """
    try:
        # Map course name to weather URL
        weather_urls = {
            "birdsfoot": "https://api.weather.gov/gridpoints/TOP/31,80/forecast",
            "totteridge": "https://api.weather.gov/gridpoints/PBZ/95,64/forecast",
        }

        course_key = course_name.lower()
        if "birdsfoot" in course_key:
            url = weather_urls["birdsfoot"]
        elif "totteridge" in course_key:
            url = weather_urls["totteridge"]
        else:
            return f"Unknown course: {course_name}. Available courses: Birdsfoot Golf Course, Totteridge"

        # Create web action request for weather
        web_action = {
            "version": "1.0",
            "url": url,
            "action": "weather",
            "arguments": {
                "days": days
            },
            "auth_config": {
                "type": "none"
            },
            "stage": STAGE,
            "message_type": "web_action"
        }

        # Publish message
        message_id = publish_to_web_actions(web_action)

        return f"Request sent to get weather forecast for {course_name} ({days} days). Message ID: {message_id}. The forecast will be available shortly."

    except Exception as e:
        logger.error(f"Error getting weather: {e}", exc_info=True)
        return f"Failed to get weather: {str(e)}"
