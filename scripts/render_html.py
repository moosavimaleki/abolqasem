#!/usr/bin/env python3
import json
import glob
import os
import tempfile
import webbrowser
from pathlib import Path
import markdown # requires pip install markdown

def main():
    sessions_dir = os.path.expanduser('~/.codex/sessions')
    session_files = glob.glob(os.path.join(sessions_dir, '**', 'rollout-*.jsonl'), recursive=True)
    
    if not session_files:
        print("هیچ سشنی یافت نشد!")
        return

    latest_file = max(session_files, key=os.path.getmtime)
    
    messages = []
    with open(latest_file, 'r', encoding='utf-8') as f:
        for line in f:
            try:
                data = json.loads(line)
                if data.get('type') == 'event_msg':
                    payload = data.get('payload', {})
                    msg_type = payload.get('type')
                    if msg_type == 'user_message':
                        messages.append(('user', payload.get('message', '')))
                    elif msg_type == 'agent_message':
                        messages.append(('agent', payload.get('message', '')))
            except json.JSONDecodeError:
                pass

    html_content = """
    <!DOCTYPE html>
    <html lang="fa" dir="rtl">
    <head>
        <meta charset="UTF-8">
        <title>نمایشگر چت کدکس</title>
        <style>
            @import url('https://cdn.jsdelivr.net/gh/rastikerdar/vazirmatn@v33.003/Vazirmatn-font-face.css');
            body {
                font-family: 'Vazirmatn', Tahoma, sans-serif;
                background-color: #f4f4f9;
                color: #333;
                line-height: 1.6;
                margin: 0;
                padding: 20px;
            }
            .chat-container {
                max-width: 800px;
                margin: auto;
                background: white;
                padding: 20px;
                border-radius: 8px;
                box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            }
            .message {
                margin-bottom: 20px;
                padding: 15px;
                border-radius: 8px;
            }
            .user {
                background-color: #e3f2fd;
                border-right: 5px solid #2196f3;
            }
            .agent {
                background-color: #f5f5f5;
                border-right: 5px solid #4caf50;
            }
            .role-title {
                font-weight: bold;
                margin-bottom: 10px;
                color: #555;
            }
            pre {
                background: #2d2d2d;
                color: #ccc;
                padding: 15px;
                border-radius: 5px;
                overflow-x: auto;
                direction: ltr;
                text-align: left;
            }
            code {
                font-family: Consolas, Monaco, 'Andale Mono', 'Ubuntu Mono', monospace;
            }
            table {
                width: 100%;
                border-collapse: collapse;
                margin: 15px 0;
            }
            th, td {
                border: 1px solid #ddd;
                padding: 8px;
            }
            th {
                background-color: #f2f2f2;
            }
        </style>
    </head>
    <body>
        <div class="chat-container">
            <h2 style="text-align:center;">چت اخیر کدکس</h2>
    """

    md = markdown.Markdown(extensions=['fenced_code', 'tables'])

    for role, text in messages:
        if not text.strip():
            continue
        html_content += f'<div class="message {role}">'
        role_fa = "شما" if role == 'user' else "کدکس"
        html_content += f'<div class="role-title">{role_fa}</div>'
        html_text = md.convert(text)
        html_content += f'<div class="content">{html_text}</div>'
        html_content += '</div>'

    html_content += """
        </div>
    </body>
    </html>
    """

    temp_html = os.path.join(tempfile.gettempdir(), 'codex_chat_rtl.html')
    with open(temp_html, 'w', encoding='utf-8') as f:
        f.write(html_content)

    webbrowser.open('file://' + temp_html)
    print(f"Chat opened in browser from {temp_html}")

if __name__ == '__main__':
    main()
