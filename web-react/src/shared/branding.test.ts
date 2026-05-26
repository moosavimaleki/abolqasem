import { describe, expect, test } from "bun:test"
import {
  CLI_COMMAND,
  getDataDir,
  getDataDirDisplay,
  getDataRootName,
  getKeybindingsFilePath,
  getKeybindingsFilePathDisplay,
  getCliInvocation,
  LOCAL_UI_URL,
  getRuntimeProfile,
} from "./branding"

describe("cli branding", () => {
  test("uses the abolqasem command shown by the installed binary", () => {
    expect(CLI_COMMAND).toBe("abolqasem")
    expect(getCliInvocation("open")).toBe("abolqasem open")
    expect(getCliInvocation("service start")).toBe("abolqasem service start")
    expect(LOCAL_UI_URL).toBe("http://127.0.0.1:9090/")
  })
})

describe("runtime profile helpers", () => {
  test("defaults to the prod profile when unset", () => {
    expect(getRuntimeProfile({})).toBe("prod")
    expect(getDataRootName({})).toBe(".abolqasem")
    expect(getDataDir("/tmp/home", {})).toBe("/tmp/home/.abolqasem/data")
    expect(getDataDirDisplay({})).toBe("~/.abolqasem/data")
    expect(getKeybindingsFilePath("/tmp/home", {})).toBe("/tmp/home/.abolqasem/keybindings.json")
    expect(getKeybindingsFilePathDisplay({})).toBe("~/.abolqasem/keybindings.json")
  })

  test("switches to dev paths for the dev profile", () => {
    const env = { ABOLQASEM_RUNTIME_PROFILE: "dev" }

    expect(getRuntimeProfile(env)).toBe("dev")
    expect(getDataRootName(env)).toBe(".abolqasem-dev")
    expect(getDataDir("/tmp/home", env)).toBe("/tmp/home/.abolqasem-dev/data")
    expect(getDataDirDisplay(env)).toBe("~/.abolqasem-dev/data")
    expect(getKeybindingsFilePath("/tmp/home", env)).toBe("/tmp/home/.abolqasem-dev/keybindings.json")
    expect(getKeybindingsFilePathDisplay(env)).toBe("~/.abolqasem-dev/keybindings.json")
  })
})
