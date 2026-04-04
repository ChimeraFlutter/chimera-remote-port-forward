#!/usr/bin/env python3
"""测试端口转发连接"""

import socket
import sys

def test_tcp(host, port, message=b"Hello from test"):
    print(f"Testing TCP connection to {host}:{port}")

    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(5)
        sock.connect((host, port))
        print(f"[OK] Connected to {host}:{port}")

        sock.send(message)
        print(f"[OK] Sent: {message}")

        try:
            response = sock.recv(1024)
            print(f"[OK] Received: {response}")
        except socket.timeout:
            print("[WARN] No response (timeout)")

        sock.close()
        return True

    except socket.timeout:
        print(f"[FAIL] Connection timeout")
        return False
    except ConnectionRefusedError:
        print(f"[FAIL] Connection refused")
        return False
    except Exception as e:
        print(f"[FAIL] Error: {e}")
        return False

def test_ws(host, port, path="/h5"):
    """测试 WebSocket 连接"""
    import urllib.request
    import urllib.error

    url = f"ws://{host}:{port}{path}"
    print(f"Testing WebSocket connection to {url}")

    try:
        # 使用 websocket-client 库（如果已安装）
        import websocket
        ws = websocket.create_connection(url, timeout=5)
        print(f"[OK] WebSocket connected to {url}")

        # 发送测试消息
        test_msg = '{"type":"ping"}'
        ws.send(test_msg)
        print(f"[OK] Sent: {test_msg}")

        try:
            response = ws.recv()
            print(f"[OK] Received: {response}")
        except:
            print("[WARN] No response")

        ws.close()
        return True
    except ImportError:
        print("[INFO] websocket-client not installed, trying HTTP...")
        # 回退到 HTTP 测试
        http_url = f"http://{host}:{port}{path}"
        try:
            req = urllib.request.Request(http_url, headers={"Connection": "Upgrade"})
            resp = urllib.request.urlopen(req, timeout=5)
            print(f"[OK] HTTP Response: {resp.status}")
            return True
        except urllib.error.HTTPError as e:
            # 400/426 错误是正常的（期望 WebSocket 升级）
            if e.code in [400, 426]:
                print(f"[OK] Server expects WebSocket upgrade (HTTP {e.code})")
                return True
            print(f"[FAIL] HTTP Error: {e.code} {e.reason}")
            return False
        except Exception as e:
            print(f"[FAIL] Error: {e}")
            return False
    except Exception as e:
        print(f"[FAIL] WebSocket Error: {e}")
        return False

if __name__ == "__main__":
    host = sys.argv[1] if len(sys.argv) > 1 else "localhost"
    port = int(sys.argv[2]) if len(sys.argv) > 2 else 10000

    # 测试 TCP 连通性
    test_tcp(host, port)
    print()

    # 测试 WebSocket
    test_ws(host, port)
