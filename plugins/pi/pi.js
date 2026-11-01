// Hypercube extension for Pi: it registers the hypercube MCP server that the
// plugin describes in HYPERCUBE_MCP.
export default function (pi) {
  pi.registerMcpServer("hypercube", JSON.parse(process.env.HYPERCUBE_MCP))
}
