FROM alpine:3.20 AS build
RUN apk add --no-cache hugo
WORKDIR /src
COPY . .
RUN hugo --minify && chmod -R a+rX /src/public

FROM nginx:alpine
COPY --from=build /src/public/ /usr/share/nginx/html/
RUN chmod -R a+rX /usr/share/nginx/html
