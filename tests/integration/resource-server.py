"""Local HTTPS and DNS fixtures. Never contacts external services."""

import http.server
import ipaddress
import pathlib
import socket
import ssl
import struct
import threading


def address(index, maximum):
    if maximum:
        return ipaddress.IPv6Address(int(ipaddress.IPv6Address("2606:4700:50::1")) + index * 65536)
    return ipaddress.IPv4Address(int(ipaddress.IPv4Address("93.184.0.1")) + index * 256)


def dns():
    connection = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    connection.bind(("127.0.0.1", 53))
    while True:
        query, peer = connection.recvfrom(4096)
        end, labels = 12, []
        while query[end]:
            length = query[end]
            labels.append(query[end + 1 : end + length + 1].decode())
            end += length + 1
        kind = struct.unpack_from("!H", query, end + 1)[0]
        end += 5
        maximum = pathlib.Path("/tmp/resource-mode").read_text().strip() == "maximum"
        count = 16 if maximum else 4
        records = count if kind == (28 if maximum else 1) else 0
        index = int(labels[0][1:])
        header = struct.pack("!HHHHHH", struct.unpack_from("!H", query)[0], 0x8180, 1, records, 0, 0)
        response = header + query[12:end]
        for offset in range(records):
            packed = address(index * count + offset, maximum).packed
            response += struct.pack("!HHHIH", 0xC00C, kind, 1, 60, len(packed)) + packed
        connection.sendto(response, peer)


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        content = pathlib.Path("/tmp/resource-list").read_bytes()
        self.send_response(200)
        self.send_header("Content-Length", str(len(content)))
        self.end_headers()
        self.wfile.write(content)

    def log_message(self, *_):
        pass


if __name__ == "__main__":
    threading.Thread(target=dns, daemon=True).start()
    server = http.server.HTTPServer(("127.0.0.1", 443), Handler)
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain("/tmp/resource-cert.pem", "/tmp/resource-key.pem")
    server.socket = context.wrap_socket(server.socket, server_side=True)
    pathlib.Path("/tmp/resource-ready").touch()
    server.serve_forever()
