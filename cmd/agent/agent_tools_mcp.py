"""
Agent Tools using MCP Server
Tools that the AI agent can use via the centralized MCP server.
"""
import logging
from typing import Any, Dict, Optional

from langchain_core.tools import tool
from mcp_client import create_mcp_client

# Configure logging
logger = logging.getLogger()

# Create MCP client (will be initialized lazily)
_mcp_client = None


def get_mcp_client():
    """Get or create the MCP client"""
    global _mcp_client
    if _mcp_client is None:
        _mcp_client = create_mcp_client()
        if _mcp_client is None:
            raise Exception("MCP client not configured. Set MCP_SERVER_URL environment variable.")
    return _mcp_client


@tool
def send_notification_tool(title: str, message: str, priority: str = "default") -> str:
    """
    Send a push notification to the user via ntfy.

    Args:
        title: The notification title (optional)
        message: The notification message to send
        priority: Priority level (low, default, high), defaults to "default"

    Returns:
        Confirmation message
    """
    try:
        client = get_mcp_client()

        result = client.call_tool('send_push_notification', {
            'title': title,
            'message': message,
            'priority': priority
        })

        logger.info(f"Notification sent via MCP: {title}")
        return result

    except Exception as e:
        logger.error(f"Error sending notification via MCP: {e}", exc_info=True)
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
        client = get_mcp_client()

        result = client.call_tool('golf_get_reservations', {
            'course_name': course_name
        })

        logger.info(f"Retrieved reservations via MCP for {course_name}")
        return result

    except Exception as e:
        logger.error(f"Error getting reservations via MCP: {e}", exc_info=True)
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

        client = get_mcp_client()

        # Format datetime strings
        start_datetime = f"{date}T{start_time}:00"
        end_datetime = f"{date}T{end_time}:00" if end_time else f"{date}T23:59:00"

        result = client.call_tool('golf_search_tee_times', {
            'course_name': course_name,
            'start_time': start_datetime,
            'end_time': end_datetime,
            'num_players': number_of_players,
            'auto_book': auto_book
        })

        logger.info(f"Searched tee times via MCP for {course_name} on {date}")
        return result

    except Exception as e:
        logger.error(f"Error searching tee times via MCP: {e}", exc_info=True)
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
        client = get_mcp_client()

        result = client.call_tool('golf_book_tee_time', {
            'course_name': course_name,
            'tee_sheet_id': tee_sheet_id
        })

        logger.info(f"Booked tee time via MCP: {tee_sheet_id} at {course_name}")
        return result

    except Exception as e:
        logger.error(f"Error booking tee time via MCP: {e}", exc_info=True)
        return f"Failed to book tee time: {str(e)}"


@tool
def get_weather_tool(location: str, days: int = 2) -> str:
    """
    Get weather forecast for a location.

    Args:
        location: Weather.gov API forecast URL or course name
        days: Number of days to forecast (default: 2, max: 7)

    Returns:
        Weather forecast information
    """
    try:
        # Validate days
        if days < 1 or days > 7:
            return "Number of days must be between 1 and 7"

        client = get_mcp_client()

        # If location is a course name, map it to weather URL
        weather_urls = {
            "birdsfoot": "https://api.weather.gov/gridpoints/TOP/31,80/forecast",
            "totteridge": "https://api.weather.gov/gridpoints/PBZ/95,64/forecast",
        }

        location_key = location.lower()
        if "birdsfoot" in location_key:
            weather_url = weather_urls["birdsfoot"]
        elif "totteridge" in location_key:
            weather_url = weather_urls["totteridge"]
        elif location.startswith("http"):
            # Already a URL
            weather_url = location
        else:
            return f"Unknown location: {location}. Provide a weather.gov API URL or course name (Birdsfoot, Totteridge)"

        result = client.call_tool('get_weather', {
            'location': weather_url,
            'days': days
        })

        logger.info(f"Retrieved weather via MCP for {location}")
        return result

    except Exception as e:
        logger.error(f"Error getting weather via MCP: {e}", exc_info=True)
        return f"Failed to get weather: {str(e)}"
