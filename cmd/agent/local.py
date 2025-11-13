from flask import Flask, request, jsonify
import json, logging, asyncio

app = Flask(__name__)

# Import your Lambda handler function
from main import async_lambda_handler

@app.route(path='/agent', methods=['POST'])
async def serve_js_handler(path):
    # Simulate API Gateway event
    event = {
        "httpMethod": request.method,
        "rawPath": f"/agent",
        "path": f"/{path}",
        "queryStringParameters": request.args,
        "headers": dict(request.headers),
        "body": request.data,
        "isBase64Encoded": False # Adjust if your Lambda expects base64 encoded body
    }

    # Mock context object (can be expanded for more advanced testing)
    context = {}
    logging.info(f"rawPath: {event['rawPath']}")
    logging.info(f"path: {event['path']}")
    
    # Invoke your Lambda handler
    lambda_response = await async_lambda_handler(event, context)
    
    body = lambda_response["body"]
    status_code = lambda_response["statusCode"]
    hdrs = lambda_response["headers"]
    
    return body, status_code, hdrs

@app.route(path='/agent/ui', methods=['GET'])
async def serve_lambda_handler(path):
    # Simulate API Gateway event
    event = {
        "httpMethod": request.method,
        "rawPath": f"/agent/ui",
        "path": f"/{path}",
        "queryStringParameters": request.args,
        "headers": dict(request.headers),
        "body": request.data.decode('utf-8') if request.data else json.dumps({'message': 'Body successfully processed'}),
        "isBase64Encoded": False # Adjust if your Lambda expects base64 encoded body
    }

    # Mock context object (can be expanded for more advanced testing)
    context = {}
    logging.info(f"rawPath: {event['rawPath']}")
    logging.info(f"path: {event['path']}")
    # Invoke your Lambda handler
    lambda_response = await async_lambda_handler(event, context)

    #logging.info(f"Lambda response: {lambda_response}")

    # Construct Flask response from Lambda response
    #response = jsonify(json.loads(lambda_response["body"]))
    body = lambda_response["body"]
    status_code = lambda_response["statusCode"]
    hdrs = lambda_response["headers"]
    #for header, value in hdrs.items():
    #    headers[header] = value
    
    return body, status_code, hdrs

if __name__ == '__main__':
    app.run(debug=True, port=5000)