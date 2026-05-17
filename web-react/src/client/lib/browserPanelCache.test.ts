import { beforeEach, describe, expect, test } from "bun:test"
import type { AbolqasemSocket } from "../app/socket"
import type { LocalHttpServerInfo } from "../../shared/protocol"
import {
  getCachedLocalHttpServers,
  refreshCachedLocalHttpServers,
  removeCachedLocalHttpServer,
} from "./browserPanelCache"

function createSocket(responses: Record<string, LocalHttpServerInfo[]>): AbolqasemSocket {
  return {
    command: (command: { projectId?: string }) => Promise.resolve(responses[command.projectId ?? "__global__"] ?? []),
  } as unknown as AbolqasemSocket
}

describe("browserPanelCache", () => {
  beforeEach(() => {
    removeCachedLocalHttpServer(3000, "project-1")
    removeCachedLocalHttpServer(4000, "project-2")
  })

  test("keeps local HTTP server caches scoped per project", async () => {
    const socket = createSocket({
      "project-1": [{ title: "one", address: "http://127.0.0.1:3000", port: 3000, status: 200, sameProject: true }],
      "project-2": [{ title: "two", address: "http://127.0.0.1:4000", port: 4000, status: 200, sameProject: true }],
    })

    await refreshCachedLocalHttpServers(socket, "project-1")
    await refreshCachedLocalHttpServers(socket, "project-2")

    expect(getCachedLocalHttpServers("project-1")?.map((server) => server.port)).toEqual([3000])
    expect(getCachedLocalHttpServers("project-2")?.map((server) => server.port)).toEqual([4000])

    removeCachedLocalHttpServer(3000, "project-1")

    expect(getCachedLocalHttpServers("project-1")).toEqual([])
    expect(getCachedLocalHttpServers("project-2")?.map((server) => server.port)).toEqual([4000])
  })
})
