"""PyGo hello-world example — Python domain runtime.

Serves handlers via UDS + MessagePack. Go web layer calls:
  pygoApp.Call("hello.greet", {"name": "World"})
"""
import asyncio
import os
import socket

import msgpack

HANDLERS = {}


def register(name):
    """Decorator to register a handler by qualified name."""
    def decorator(func):
        HANDLERS[name] = func
        return func
    return decorator


@register("hello.greet")
def greet(name="World"):
    """Say hello — called from Go via bridge."""
    return {"message": f"Hello, {name}!", "framework": "PyGo native dual-language"}


def handle_request(payload: bytes) -> bytes:
    """Process a single msgpack request frame."""
    req = msgpack.unpackb(payload, raw=False, strict_map_key=False)
    method = req.get("method", "")
    args = req.get("args", {}) or {}

    fn = HANDLERS.get(method)
    if fn is None:
        resp = {"result": None, "error": f"Handler not found: {method}"}
    else:
        try:
            result = fn(**args)
            resp = {"result": result, "error": None}
        except Exception as e:
            import traceback
            resp = {"result": None, "error": f"{e}\n{traceback.format_exc()}"}

    return msgpack.packb(resp, use_bin_type=True)


async def start_server(socket_path):
    """Start UDS server — listens for Go bridge connections."""
    try:
        os.unlink(socket_path)
    except FileNotFoundError:
        pass
    os.makedirs(os.path.dirname(socket_path) or ".", exist_ok=True)

    async def handle_conn(reader, writer):
        try:
            while True:
                header = await reader.readexactly(4)
                length = int.from_bytes(header, byteorder="big")
                if length == 0:
                    continue
                payload = await reader.readexactly(length)
                response = handle_request(payload)
                resp_header = len(response).to_bytes(4, byteorder="big")
                writer.write(resp_header + response)
                await writer.drain()
        except (asyncio.IncompleteReadError, ConnectionResetError, BrokenPipeError):
            pass
        finally:
            writer.close()

    server = await asyncio.start_unix_server(handle_conn, path=socket_path)
    os.chmod(socket_path, 0o666)
    print(f"PyGo Python domain server ready on {socket_path}", flush=True)
    print(f"Handlers: {list(HANDLERS.keys())}", flush=True)

    async with server:
        await server.serve_forever()


def main():
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("--socket", default="storage/.pygo.sock")
    args = parser.parse_args()
    asyncio.run(start_server(args.socket))


if __name__ == "__main__":
    main()
