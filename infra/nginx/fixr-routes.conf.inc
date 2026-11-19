add_header X-Request-Id $request_id always;

location = /healthz { return 200 "ok\n"; }

location = /api/auth/otp {
  limit_req zone=otp burst=3 nodelay;
  proxy_pass http://order_service;
}

location /api/payments/ {
  limit_req zone=api burst=20 nodelay;
  proxy_pass http://payments_service;
}

location /api/ {
  limit_req zone=api burst=80 nodelay;
  proxy_pass http://order_service;
}

location /.well-known/ { proxy_pass http://order_service; }

location /loc/ {
  limit_req zone=loc burst=40 nodelay;
  proxy_pass http://location_ingest;
}

location /dispatch/ {
  limit_req zone=api burst=40 nodelay;
  proxy_pass http://dispatch_service;
}

location /pay/ { proxy_pass http://payments_service; }

location /ai/ {
  limit_req zone=api burst=20 nodelay;
  proxy_read_timeout 90s;
  proxy_pass http://ai_service;
}

location /ws {
  proxy_set_header Host $host;
  proxy_set_header X-Request-Id $request_id;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header Upgrade $http_upgrade;
  proxy_set_header Connection $connection_upgrade;
  proxy_read_timeout 1h;
  proxy_send_timeout 1h;
  proxy_pass http://realtime_gateway;
}

location /media/ {
  proxy_set_header Host minio:9000;
  proxy_set_header Connection "";
  proxy_pass http://minio:9000/;
}
