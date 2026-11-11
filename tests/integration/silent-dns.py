"""Keep a worker inside a real DNS operation until the interruption test kills it."""

import pathlib
import socket

connection = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
connection.bind(("127.0.0.1", 53))
pathlib.Path("/tmp/silent-dns-ready").touch()
while True:
    connection.recvfrom(4096)
