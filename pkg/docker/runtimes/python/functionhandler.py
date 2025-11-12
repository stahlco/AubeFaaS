from time import sleep

from websockets.sync.server import serve
import os

def print_tree(startpath, prefix=""):
    entries = sorted(os.listdir(startpath))
    for i, entry in enumerate(entries):
        path = os.path.join(startpath, entry)
        connector = "└── " if i == len(entries) - 1 else "├── "
        print(prefix + connector + entry)
        if os.path.isdir(path):
            extension = "    " if i == len(entries) - 1 else "│   "
            print_tree(path, prefix + extension)


if __name__ == "__main__":
    print_tree(".")
    # Import the function
    sleep(10)
    try:
        from fn.fn import fn
    except ImportError:
        raise ImportError("Failed to import fn.py")


    def function_handler(websocket) -> None:
        try:
            fn(websocket)
        except Exception as e:
            websocket.send(f"Failed to call function: {str(e)}")

    # You could another HandlerClass with manages health-checks (but I don´t care rn)

    with serve(function_handler, "", 8000) as server:
        print("Server running")
        server.serve_forever()