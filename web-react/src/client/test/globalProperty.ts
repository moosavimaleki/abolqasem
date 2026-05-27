export function replaceGlobalProperty(name: PropertyKey, value: unknown) {
  const descriptor = Object.getOwnPropertyDescriptor(globalThis, name)
  Object.defineProperty(globalThis, name, {
    configurable: true,
    writable: true,
    value,
  })
  return () => {
    if (descriptor) {
      Object.defineProperty(globalThis, name, descriptor)
      return
    }
    delete (globalThis as typeof globalThis & Record<PropertyKey, unknown>)[name]
  }
}

export function restoreGlobalProperties(restores: Array<() => void>) {
  for (let index = restores.length - 1; index >= 0; index--) {
    restores[index]?.()
  }
}
