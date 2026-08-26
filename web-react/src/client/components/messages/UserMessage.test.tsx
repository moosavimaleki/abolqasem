import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { UserMessage } from "./UserMessage"

describe("UserMessage", () => {
  test("renders internal transcript payloads collapsed instead of hiding them", () => {
    const html = renderToStaticMarkup(
      <UserMessage content={"<environment_context>\n<cwd>/tmp/project</cwd>\n</environment_context>"} />,
    )

    expect(html).toContain("<details")
    expect(html).toContain("زمینهٔ محیط اجرا")
    expect(html).toContain("&lt;environment_context&gt;")
	    expect((html.match(/&lt;environment_context&gt;/g) ?? [])).toHaveLength(1)
  })

  test("renders aborted turns as collapsed system events", () => {
    const html = renderToStaticMarkup(
      <UserMessage content={"<turn_aborted>\nThe user interrupted the previous turn.\n</turn_aborted>"} />,
    )

    expect(html).toContain("<details")
    expect(html).toContain("رخداد سیستمی")
    expect(html).toContain("&lt;turn_aborted&gt;")
	    expect((html.match(/&lt;turn_aborted&gt;/g) ?? [])).toHaveLength(1)
  })

  test("keeps surrounding user content without repeating the collapsed payload", () => {
    const html = renderToStaticMarkup(
      <UserMessage content={"قبل\n<environment_context>\n<cwd>/tmp/project</cwd>\n</environment_context>\nبعد"} />,
    )

    expect(html).toContain("قبل")
    expect(html).toContain("بعد")
    expect((html.match(/&lt;environment_context&gt;/g) ?? [])).toHaveLength(1)
  })
})
