def fn(websocket):
    # process the msg in some manner
    for msg in websocket:
        websocket.send(str(msg)[::-1])