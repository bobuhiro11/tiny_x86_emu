#!/usr/bin/python
import SimpleHTTPServer
import SocketServer
import mimetypes

Handler = SimpleHTTPServer.SimpleHTTPRequestHandler
Handler.extensions_map['.wasm']='application/wasm'
httpd = SocketServer.TCPServer(("", 8000), Handler)

print "serving at port", 8000
httpd.serve_forever()
