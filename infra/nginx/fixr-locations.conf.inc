proxy_http_version 1.1;
proxy_set_header Host $host;
proxy_set_header Connection "";
proxy_set_header X-Request-Id $request_id;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $scheme;
proxy_next_upstream error timeout http_502 http_503;
proxy_next_upstream_tries 2;
