"""
MCP Client for Python
Provides a simple interface to communicate with the MCP server.
"""
import json
import logging
import os
from typing import Any, Dict, List, Optional
import urllib.request
import urllib.error

logger = logging.getLogger(__name__)


class MCPClient:
    """Client for communicating with MCP server via JSON-RPC 2.0"""

    def __init__(self, server_url: str, api_key: Optional[str] = None):
        """
        Initialize MCP client.

        Args:
            server_url: URL of the MCP server endpoint
            api_key: Optional API key for authentication
        """
        self.server_url = server_url
        self.api_key = api_key
        self.initialized = False
        self.request_id = 0

    def _get_request_id(self) -> str:
        """Generate a unique request ID"""
        self.request_id += 1
        return str(self.request_id)

    def _make_request(self, method: str, params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """
        Make a JSON-RPC request to the MCP server.

        Args:
            method: JSON-RPC method name
            params: Optional parameters for the method

        Returns:
            Response from the server

        Raises:
            Exception: If the request fails or returns an error
        """
        request_payload = {
            "jsonrpc": "2.0",
            "id": self._get_request_id(),
            "method": method,
        }

        if params is not None:
            request_payload["params"] = params

        request_data = json.dumps(request_payload).encode('utf-8')

        # Create request
        req = urllib.request.Request(
            self.server_url,
            data=request_data,
            headers={
                'Content-Type': 'application/json',
            }
        )

        # Add API key if provided
        if self.api_key:
            req.add_header('X-API-Key', self.api_key)

        try:
            logger.info(f"Making MCP request: {method}")

            with urllib.request.urlopen(req, timeout=30) as response:
                response_data = response.read().decode('utf-8')
                response_json = json.loads(response_data)

                logger.info(f"MCP response received for {method}")

                # Check for JSON-RPC error
                if 'error' in response_json:
                    error = response_json['error']
                    error_msg = f"MCP Error {error.get('code')}: {error.get('message')}"
                    if 'data' in error:
                        error_msg += f" - {error['data']}"
                    logger.error(error_msg)
                    raise Exception(error_msg)

                return response_json.get('result', {})

        except urllib.error.HTTPError as e:
            error_msg = f"HTTP Error {e.code}: {e.reason}"
            logger.error(error_msg)
            raise Exception(error_msg)
        except urllib.error.URLError as e:
            error_msg = f"URL Error: {e.reason}"
            logger.error(error_msg)
            raise Exception(error_msg)
        except json.JSONDecodeError as e:
            error_msg = f"Invalid JSON response: {e}"
            logger.error(error_msg)
            raise Exception(error_msg)

    def initialize(self) -> Dict[str, Any]:
        """
        Initialize connection with the MCP server.

        Returns:
            Server capabilities and information
        """
        if self.initialized:
            return {"status": "already_initialized"}

        result = self._make_request('initialize', {
            'protocolVersion': '2025-03-26',
            'capabilities': {},
            'clientInfo': {
                'name': 'rez-agent-python-client',
                'version': '1.0.0'
            }
        })

        self.initialized = True
        logger.info("MCP client initialized successfully")
        return result

    def list_tools(self) -> List[Dict[str, Any]]:
        """
        List all available tools from the MCP server.

        Returns:
            List of tool definitions
        """
        if not self.initialized:
            self.initialize()

        result = self._make_request('tools/list')
        return result.get('tools', [])

    def call_tool(self, tool_name: str, arguments: Dict[str, Any]) -> str:
        """
        Call a tool on the MCP server.

        Args:
            tool_name: Name of the tool to call
            arguments: Arguments to pass to the tool

        Returns:
            Tool execution result as a formatted string
        """
        if not self.initialized:
            self.initialize()

        result = self._make_request('tools/call', {
            'name': tool_name,
            'arguments': arguments
        })

        # Extract content from response
        content = result.get('content', [])
        if not content:
            return "Tool executed successfully (no output)"

        # Format content items into a readable string
        output_parts = []
        for item in content:
            if item.get('type') == 'text':
                output_parts.append(item.get('text', ''))

        return '\n'.join(output_parts) if output_parts else "Tool executed successfully"

    def ping(self) -> Dict[str, Any]:
        """
        Ping the MCP server to check connectivity.

        Returns:
            Ping response
        """
        return self._make_request('ping')


def create_mcp_client() -> Optional[MCPClient]:
    """
    Create an MCP client from environment variables.

    Environment variables:
        MCP_SERVER_URL: URL of the MCP server
        MCP_API_KEY: Optional API key for authentication

    Returns:
        MCPClient instance or None if not configured
    """
    server_url = os.environ.get('MCP_SERVER_URL')
    if not server_url:
        logger.warning("MCP_SERVER_URL not configured, MCP client unavailable")
        return None

    api_key = os.environ.get('MCP_API_KEY')

    client = MCPClient(server_url, api_key)

    # Initialize on creation
    try:
        client.initialize()
        logger.info("MCP client created and initialized")
        return client
    except Exception as e:
        logger.error(f"Failed to initialize MCP client: {e}")
        return None
