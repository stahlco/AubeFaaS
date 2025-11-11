ARG PYTHON_VERSION=3.11
ARG ALPINE_VERSION=3.19

FROM ghcr.io/astral-sh/uv:python3.11-alpine

# Create app directory
WORKDIR /usr/src/app

COPY functionhandler.py .