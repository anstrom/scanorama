FROM ubuntu:22.04

# Avoid prompts from apt and ensure pip doesn't use cache
ENV DEBIAN_FRONTEND=noninteractive \
    PIP_NO_CACHE_DIR=1


# Install required packages
RUN apt-get update && apt-get install -y \
    nginx \
    python3 \
    python3-pip \
    openssh-server \
    redis-server \
    netcat-openbsd \
    curl \
    && rm -rf /var/lib/apt/lists/* \
    && pip3 install --upgrade pip

# Set up SSH
RUN mkdir /var/run/sshd
RUN echo 'root:testpassword' | chpasswd
RUN sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config

# Set up Nginx
COPY nginx.conf /etc/nginx/sites-available/default

# Set up Redis
RUN sed -i 's/bind 127.0.0.1 ::1/bind 0.0.0.0/' /etc/redis/redis.conf

# Set up Python test server
COPY requirements.txt /app/
WORKDIR /app
# hadolint ignore=DL3013
RUN pip3 install -r requirements.txt
# hadolint ignore=DL3059
COPY app.py .

# Expose ports
EXPOSE 22
EXPOSE 80
EXPOSE 443
EXPOSE 6379
EXPOSE 8080

# Start services
COPY start-services.sh /start-services.sh
RUN chmod +x /start-services.sh
CMD ["/start-services.sh"]
