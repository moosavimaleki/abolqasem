import type { ClientEnvelope, ServerEnvelope } from "../../shared/protocol"

type WorkerRequest =
  | { type: "start"; url: string }
  | { type: "send"; envelope: ClientEnvelope }
  | { type: "cancel-command"; id: string }
  | { type: "reconnect" }
  | { type: "dispose" }

type WorkerResponse = { type: "status"; status: "connecting" | "connected" | "disconnected" } | { type: "message"; payload: string }
type SharedWorkerStatus = "connecting" | "connected" | "disconnected"
interface SharedWorkerScopeLike {
  onconnect: ((event: MessageEvent) => void) | null
}

const RECONNECT_INITIAL_DELAY_MS = 750
const RECONNECT_MAX_DELAY_MS = 5_000

const workerScope = self as unknown as SharedWorkerScopeLike
const ports = new Set<MessagePort>()
const subscriptionOwners = new Map<string, MessagePort>()
const commandOwners = new Map<string, MessagePort>()
const subscriptions = new Map<string, ClientEnvelope>()
const outboundQueue: ClientEnvelope[] = []

let socket: WebSocket | null = null
let socketURL = ""
let reconnectTimer: number | null = null
let reconnectDelayMs = RECONNECT_INITIAL_DELAY_MS
let status: SharedWorkerStatus = "disconnected"

workerScope.onconnect = (event: MessageEvent) => {
  const port = event.ports[0]
  if (!port) return
  port.start()
  port.addEventListener("message", (messageEvent: MessageEvent<WorkerRequest>) => {
    handlePortMessage(port, messageEvent.data)
  })
}

function handlePortMessage(port: MessagePort, message: WorkerRequest) {
  switch (message.type) {
    case "start":
      ports.add(port)
      socketURL = message.url
      post(port, { type: "status", status })
      connect()
      return
    case "send":
      trackEnvelope(port, message.envelope)
      send(message.envelope)
      return
    case "cancel-command":
      commandOwners.delete(message.id)
      discardQueuedCommand(message.id)
      return
    case "reconnect":
      reconnectNow()
      return
    case "dispose":
      disposePort(port)
      return
  }
}

function trackEnvelope(port: MessagePort, envelope: ClientEnvelope) {
  if (!envelope.id) return
  if (envelope.type === "subscribe") {
    subscriptionOwners.set(envelope.id, port)
    subscriptions.set(envelope.id, envelope)
    return
  }
  if (envelope.type === "unsubscribe") {
    subscriptionOwners.delete(envelope.id)
    subscriptions.delete(envelope.id)
    return
  }
  if (envelope.type === "command") {
    commandOwners.set(envelope.id, port)
  }
}

function disposePort(port: MessagePort) {
  ports.delete(port)
  for (const [subscriptionID, owner] of subscriptionOwners) {
    if (owner !== port) continue
    subscriptionOwners.delete(subscriptionID)
    subscriptions.delete(subscriptionID)
    send({ v: 1, type: "unsubscribe", id: subscriptionID })
  }
  for (const [commandID, owner] of commandOwners) {
    if (owner !== port) continue
    commandOwners.delete(commandID)
    discardQueuedCommand(commandID)
  }
  port.close()
  if (ports.size === 0) closeSocket()
}

function connect() {
  if (!socketURL || ports.size === 0 || socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) return
  setStatus("connecting")
  socket = new WebSocket(socketURL)
  socket.addEventListener("open", () => {
    reconnectDelayMs = RECONNECT_INITIAL_DELAY_MS
    setStatus("connected")
    for (const envelope of subscriptions.values()) sendNow(envelope)
    while (outboundQueue.length > 0) {
      const envelope = outboundQueue.shift()
      if (!envelope || (envelope.type === "command" && !commandOwners.has(envelope.id))) continue
      sendNow(envelope)
    }
  })
  socket.addEventListener("message", (event) => routeServerMessage(String(event.data)))
  socket.addEventListener("close", () => {
    socket = null
    commandOwners.clear()
    setStatus("disconnected")
    scheduleReconnect()
  })
  socket.addEventListener("error", () => socket?.close())
}

function closeSocket() {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  const currentSocket = socket
  socket = null
  currentSocket?.close()
  setStatus("disconnected")
}

function reconnectNow() {
  if (reconnectTimer !== null) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  if (!socket || socket.readyState === WebSocket.CLOSED) {
    connect()
    return
  }
  if (socket.readyState === WebSocket.CONNECTING) return
  socket.close()
}

function scheduleReconnect() {
  if (reconnectTimer !== null || ports.size === 0) return
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connect()
    reconnectDelayMs = Math.min(reconnectDelayMs * 2, RECONNECT_MAX_DELAY_MS)
  }, reconnectDelayMs) as unknown as number
}

function send(envelope: ClientEnvelope) {
  if (socket?.readyState === WebSocket.OPEN) {
    sendNow(envelope)
    return
  }
  if (envelope.type === "subscribe" || envelope.type === "unsubscribe") {
    connect()
    return
  }
  outboundQueue.push(envelope)
  connect()
}

function sendNow(envelope: ClientEnvelope) {
  socket?.send(JSON.stringify(envelope))
}

function discardQueuedCommand(commandID: string) {
  for (let index = outboundQueue.length - 1; index >= 0; index--) {
    const envelope = outboundQueue[index]
    if (envelope?.type === "command" && envelope.id === commandID) outboundQueue.splice(index, 1)
  }
}

function routeServerMessage(payload: string) {
  let envelope: ServerEnvelope
  try {
    envelope = JSON.parse(payload) as ServerEnvelope
  } catch {
    return
  }
  if (!envelope.id) return
  const owner = envelope.type === "snapshot" || envelope.type === "event" ? subscriptionOwners.get(envelope.id) : commandOwners.get(envelope.id)
  if (!owner) return
  if (envelope.type === "ack" || envelope.type === "error") commandOwners.delete(envelope.id)
  post(owner, { type: "message", payload })
}

function setStatus(nextStatus: SharedWorkerStatus) {
  status = nextStatus
  for (const port of ports) post(port, { type: "status", status })
}

function post(port: MessagePort, message: WorkerResponse) {
  port.postMessage(message)
}
