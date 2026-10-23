#!/usr/bin/env python3
"""Static file server for the kc landing site.

Threaded so concurrent package downloads do not serialise, and it refuses to
take a port that is already held by a process it does not own. Ownership is
established by the pidfile written here: a foreign listener is never killed,
the caller is told to pick another port.

Usage:
    serve.py --dir DIR --port PORT --pidfile PATH [--bind ADDR]
"""

import argparse
import os
import signal
import socket
import sys
import threading
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

# The site ships a video and a WebVTT track; the stdlib table lacks .vtt and
# would serve it as octet-stream, which browsers refuse to attach as a track.
EXTRA_TYPES = {
    ".vtt": "text/vtt; charset=utf-8",
    ".mp4": "video/mp4",
    ".webm": "video/webm",
    ".woff2": "font/woff2",
    ".wasm": "application/wasm",
    ".tar.gz": "application/gzip",
    ".txt": "text/plain; charset=utf-8",
}


class Handler(SimpleHTTPRequestHandler):
    def guess_type(self, path):
        lower = path.lower()
        for suffix, ctype in EXTRA_TYPES.items():
            if lower.endswith(suffix):
                return ctype
        return super().guess_type(path)

    def log_message(self, fmt, *args):
        sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))


def port_is_held(port, bind):
    probe = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        probe.bind((bind, port))
        return False
    except OSError:
        return True
    finally:
        probe.close()


def write_pidfile(path):
    tmp = path + ".tmp"
    with open(tmp, "w") as handle:
        handle.write("%d\n" % os.getpid())
    os.replace(tmp, path)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dir", required=True)
    parser.add_argument("--port", type=int, required=True)
    parser.add_argument("--bind", default="0.0.0.0")
    parser.add_argument("--pidfile", required=True)
    args = parser.parse_args()

    root = os.path.abspath(args.dir)
    if not os.path.isdir(root):
        sys.exit("serve: not a directory: %s" % root)
    if not os.path.isfile(os.path.join(root, "index.html")):
        sys.exit("serve: no index.html in %s" % root)
    if port_is_held(args.port, args.bind):
        sys.exit(
            "serve: %s:%d is already in use by another process; "
            "refusing to start (nothing was killed)" % (args.bind, args.port)
        )

    server = ThreadingHTTPServer(
        (args.bind, args.port), partial(Handler, directory=root)
    )
    server.daemon_threads = True
    write_pidfile(args.pidfile)

    def shutdown(signum, frame):
        threading.Thread(target=server.shutdown, daemon=True).start()

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    sys.stderr.write(
        "serve: %s on http://%s:%d (pid %d)\n"
        % (root, args.bind, args.port, os.getpid())
    )
    try:
        server.serve_forever()
    finally:
        server.server_close()
        try:
            os.unlink(args.pidfile)
        except OSError:
            pass
        sys.stderr.write("serve: stopped\n")


if __name__ == "__main__":
    main()
