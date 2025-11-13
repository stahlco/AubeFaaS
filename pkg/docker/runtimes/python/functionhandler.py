import http.server
import threading

from websockets.sync.server import serve

class HealthHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        print(f"GET {self.path}")
        if self.path == "/health":
            self.send_response(200)
            self.end_headers()
            self.wfile.write("OK".encode("utf-8"))
            print("reporting health: OK")
        else:
            self.send_response(404)
            self.end_headers()

def start_health_server(host: str = "0.0.0.0", port: int = 8080):
    s = http.server.ThreadingHTTPServer((host, port), HealthHandler)
    s.serve_forever()

if __name__ == "__main__":
    try:
        from fn import fn
    except ImportError:
        raise ImportError("Failed to import fn.py")


    def function_handler(websocket) -> None:
        print("Received request")
        try:
            fn(websocket)
        except Exception as e:
            websocket.send(f"Failed to call function: {str(e)}")

    # You could another HandlerClass with manages health-checks (but I don´t care rn) (listens to HTTP "0.0.0.0:8000/health")

    threading.Thread(target=start_health_server, daemon=True).start()

    with serve(function_handler, "0.0.0.0", 8000) as server:
        print("Server running")
        server.serve_forever()