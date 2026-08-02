import http from "node:http";

const ports = process.argv.slice(2).map(Number);
if (ports.length !== 2 || ports.some((port) => !Number.isInteger(port) || port < 1024)) {
  throw new Error("usage: node proof-server.mjs <first-port> <second-port>");
}

const servers = ports.map((port, index) => {
  const server = http.createServer((request, response) => {
    if (request.method === "GET" && request.url === "/health") {
      response.writeHead(200, { "Content-Type": "text/plain" });
      response.end("ok");
      return;
    }
    if (request.method === "POST" && request.url === "/api/devices/pair") {
      let body = "";
      request.setEncoding("utf8");
      request.on("data", (chunk) => {
        body += chunk;
        if (body.length > 64 * 1024) request.destroy();
      });
      request.on("end", () => {
        const payload = JSON.parse(body);
        if (!payload.code || !payload.client_instance_id || !payload.public_key) {
          response.writeHead(400, { "Content-Type": "text/plain" });
          response.end("incomplete pairing request");
          return;
        }
        response.writeHead(201, { "Content-Type": "application/json" });
        response.end(JSON.stringify({
          endpoint_id: `proof-endpoint-${index + 1}`,
          protocol_version: 1,
          websocket_url: `ws://127.0.0.1:${port}/api/devices/connect`,
        }));
      });
      return;
    }
    response.writeHead(404, { "Content-Type": "text/plain" });
    response.end("not found");
  });
  server.listen(port, "127.0.0.1");
  return server;
});

process.on("SIGTERM", () => {
  for (const server of servers) server.close();
});
