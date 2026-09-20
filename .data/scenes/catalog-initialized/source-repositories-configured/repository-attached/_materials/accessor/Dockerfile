FROM python:3.13-alpine
RUN apk add --no-cache mysql-client
WORKDIR /opt/accessor
COPY source.py source.json shop.py access.py collector.py observer.py ./
EXPOSE 7390 7391 7392
