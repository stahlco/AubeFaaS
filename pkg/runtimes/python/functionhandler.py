from websockets.sync.server import serve


if __name__ == "__main__":
    # Import the function
    try:
        from fn import fn
    except ImportError:
        raise ImportError("Failed to import fn.py")


    def function_handler(websocket) -> None:
        try:
            fn.fn(websocket)
        except Exception as e:
            websocket.send(f"Failed to call function: {str(e)}")

    # You could another HandlerClass with manages health-checks (but I don´t care rn)

    with serve(function_handler, "", 8000) as server:
        print("Server running")
        server.serve_forever()