#!/usr/bin/env python3
"""简单的流式调用脚本"""

import requests
import json
import sys

API_URL = "http://localhost:8080/v1/chat/completions"
API_KEY = "123"  # 与配置中的 APIKey 一致


def stream_chat():
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {API_KEY}",
    }

    payload = {
        "model": "claude-sonnet-4-6-20250514",
        "messages": [
            {"role": "user", "content": "Hello, who are you?"}
        ],
        "stream": True,
    }

    response = requests.post(
        API_URL,
        headers=headers,
        json=payload,
        stream=True,
        timeout=120
    )

    if response.status_code != 200:
        print(f"Error: {response.status_code}")
        print(response.text)
        return

    print("Response (stream):")
    for line in response.iter_lines():
        if line:
            line = line.decode("utf-8")
            if line.startswith("data: "):
                data = line[6:]
                if data == "[DONE]":
                    break
                try:
                    obj = json.loads(data)
                    delta = obj.get("choices", [{}])[0].get("delta", {})
                    content = delta.get("content", "")
                    if content:
                        print(content, end="", flush=True)
                except json.JSONDecodeError:
                    pass
    print()


if __name__ == "__main__":
    stream_chat()
