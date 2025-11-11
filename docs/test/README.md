# Web API Tests

## Send Notification

``` bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d @./docs/test/messages/web_api_send_notification.json \
  $WEBAPI_URL/api/messages
```

## Get Reservations

``` bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d @./docs/test/messages/web_api_get_reservations.json \
  $WEBAPI_URL/api/messages
```

## Get Weather

``` bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d @./docs/test/messages/web_api_weather.json \
  $WEBAPI_URL/api/messages
```

## Get Tee Times

``` bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d @./docs/test/messages/web_api_get_tee_times.json \
  $WEBAPI_URL/api/messages
```
## Get Tee Times

``` bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d @./docs/test/messages/web_api_get_tee_times.json \
  $WEBAPI_URL/api/messages
```


