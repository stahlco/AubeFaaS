from time import sleep


def fn(websocket):
    # process the msg in some manner
    for msg in websocket:
        websocket.send(str(msg)[::-1])
        sleep(2) # in seconds