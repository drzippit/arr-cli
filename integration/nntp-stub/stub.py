"""A news server that has no articles, for the integration tests.

SABnzbd only puts a Download in its History once it has tried to fetch it.
Against this server every article is missing, so each Download fails at
once instead of waiting on a server that never answers.
"""

import socketserver


class Session(socketserver.StreamRequestHandler):
    def reply(self, line):
        self.wfile.write(line.encode() + b"\r\n")

    def handle(self):
        self.reply("200 arr-cli stub news server")
        for raw in self.rfile:
            word = raw.decode(errors="replace").split(" ", 1)[0].strip().upper()
            if word == "QUIT":
                self.reply("205 bye")
                return
            if word == "AUTHINFO":
                self.reply("281 ok")
            elif word in ("ARTICLE", "BODY", "HEAD", "STAT"):
                self.reply("430 no such article")
            elif word == "CAPABILITIES":
                self.reply("101 capabilities\r\nVERSION 2\r\nREADER\r\n.")
            elif word == "MODE":
                self.reply("200 reader")
            else:
                self.reply("500 unknown command")


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


if __name__ == "__main__":
    Server(("0.0.0.0", 119), Session).serve_forever()
