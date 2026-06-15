# Статический сайт КИЦУНЭ: nginx раздаёт файлы и проксирует /api на gateway.
FROM nginx:1.27-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY . /usr/share/nginx/html
EXPOSE 80
