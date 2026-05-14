---
name: html
description: A skill to render the current chat in an RTL-friendly HTML page. Run this when the user asks to "render html" or "show rtl" or uses "@html".
---

# HTML RTL Viewer

When the user requests to see the chat in HTML, RTL, or wants a better view of tables and markdown:

1. Acknowledge their request.
2. Run the included Python script using the shell to generate and open the HTML.

```bash
python3 plugins/rtl-viewer/scripts/render_html.py
```
