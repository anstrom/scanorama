#!/usr/bin/env python3
"""Simple Flask application for testing port scanning."""

from flask import Flask, jsonify  # type: ignore

app = Flask(__name__)


@app.route("/")
def index():
    """Return basic info about the test server."""
    return jsonify(
        {
            "name": "Test Server",
            "version": "1.0",
            "status": "running",
        }
    )


@app.route("/health")
def health():
    """Health check endpoint."""
    return jsonify({"status": "healthy"})


@app.route("/version")
def version():
    """Version information endpoint."""
    return jsonify(
        {
            "version": "1.0.0",
            "api_version": "v1",
            "build": "test",
        }
    )


if __name__ == "__main__":
    # Run on all interfaces, port 8888
    app.run(host="0.0.0.0", port=8888)
