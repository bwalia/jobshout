#!/usr/bin/env python3
"""TCP relay from the runner host to the LAN registry NodePorts.

The self-hosted runner's Docker engine (Colima) can lose its route to the LAN
while the Mac itself still reaches it: containers then get "connection refused"
from every 192.168.1.x:30500 even though the runner's own curl answers 401. The
BuildKit builder is such a container, so every push failed (runs 36204498143,
36202354483). This relay listens on the host's loopback, which containers reach
through the VM's host gateway, and forwards each connection to the first
upstream that accepts it, so a flapping node is skipped per connection.

usage: registry-relay.py LISTEN_PORT HOST:PORT [HOST:PORT ...]
"""

import asyncio
import sys

CONNECT_TIMEOUT = 3


async def pipe(reader, writer):
    try:
        while data := await reader.read(1 << 16):
            writer.write(data)
            await writer.drain()
    except (ConnectionError, OSError):
        pass
    finally:
        try:
            writer.close()
        except Exception:
            pass


async def open_upstream(upstreams):
    for host, port in upstreams:
        try:
            return await asyncio.wait_for(asyncio.open_connection(host, port), CONNECT_TIMEOUT)
        except (OSError, asyncio.TimeoutError) as e:
            print(f"relay: {host}:{port} refused: {e!r}", file=sys.stderr, flush=True)
            continue
    return None


def main():
    port = int(sys.argv[1])
    upstreams = [(h, int(p)) for h, p in (a.rsplit(":", 1) for a in sys.argv[2:])]

    async def handle(client_r, client_w):
        up = await open_upstream(upstreams)
        if up is None:
            print("relay: no upstream accepted the connection", file=sys.stderr, flush=True)
            client_w.close()
            return
        await asyncio.gather(pipe(client_r, up[1]), pipe(up[0], client_w))

    async def serve():
        server = await asyncio.start_server(handle, "127.0.0.1", port)
        print(f"relay: 127.0.0.1:{port} -> {sys.argv[2:]}", flush=True)
        async with server:
            await server.serve_forever()

    asyncio.run(serve())


if __name__ == "__main__":
    main()
