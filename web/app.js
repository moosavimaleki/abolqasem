document.addEventListener('DOMContentLoaded', () => {
    let currentSessionId = null;

    async function loadSessions() {
        try {
            const response = await fetch('/api/sessions');
            const data = await response.json();
            const listEl = document.getElementById('sessions-list');
            listEl.innerHTML = '';
            
            // Sort by updated_at descending
            data.items.sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at));
            
            data.items.forEach(session => {
                const item = document.createElement('div');
                item.className = 'session-item';
                if (session.session_id === currentSessionId) {
                    item.classList.add('active');
                }
                
                const date = new Date(session.updated_at);
                const timeStr = date.toLocaleTimeString('fa-IR');
                
                item.innerHTML = `
                    <div class="session-name">${session.project_name}</div>
                    <div class="session-time">${timeStr}</div>
                `;
                
                item.onclick = () => {
                    document.querySelectorAll('.session-item').forEach(el => el.classList.remove('active'));
                    item.classList.add('active');
                    loadSession(session.session_id, session.project_name);
                };
                listEl.appendChild(item);
            });
            
            // Auto-load latest session on first load
            if (!currentSessionId && data.items.length > 0) {
                const firstSession = data.items[0];
                listEl.firstChild.classList.add('active');
                loadSession(firstSession.session_id, firstSession.project_name);
            }
        } catch (err) {
            console.error('Failed to load sessions:', err);
        }
    }

    async function loadSession(sessionId, projectName) {
        currentSessionId = sessionId;
        document.getElementById('current-project-name').innerText = projectName || sessionId;
        
        const container = document.getElementById('chat-messages');
        container.innerHTML = '<div style="text-align: center; color: #888;">در حال بارگذاری...</div>';
        
        try {
            const response = await fetch(`/api/session/${sessionId}/messages`);
            if (!response.ok) throw new Error('Session not found or error');
            const data = await response.json();
            
            container.innerHTML = '';
            
            if (data.items.length === 0) {
                container.innerHTML = '<div style="text-align: center; color: #888;">پیامی وجود ندارد.</div>';
                return;
            }
            
            data.items.forEach(msg => {
                appendMessage(msg, container);
            });
            
            // Scroll to bottom
            container.scrollTop = container.scrollHeight;
            
        } catch (err) {
            console.error('Failed to load session messages:', err);
            container.innerHTML = '<div style="text-align: center; color: red;">خطا در بارگذاری پیام‌ها.</div>';
        }
    }

    function appendMessage(msg, container) {
        const el = document.createElement('div');
        el.className = `message ${msg.role === 'user' ? 'user' : (msg.role === 'tool' ? 'tool' : 'assistant')}`;
        
        const header = document.createElement('div');
        header.className = 'message-header';
        header.innerText = msg.role;
        
        const content = document.createElement('div');
        content.className = `message-text ${msg.direction}`;
        
        // Simple escaping
        const escapedText = msg.text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
        content.innerHTML = escapedText;
        
        el.appendChild(header);
        el.appendChild(content);
        container.appendChild(el);
    }

    // Initial load
    loadSessions();

    // SSE Live Updates
    const evtSource = new EventSource('/api/events');
    let toastTimeout = null;

    evtSource.onmessage = function(event) {
        const data = JSON.parse(event.data);
        
        if (data.session_id === currentSessionId) {
            // Auto reload current session
            loadSession(currentSessionId, document.getElementById('current-project-name').innerText);
        } else {
            // Show toast notification
            showToast(data.session_id, data.project_name);
        }
        
        // Refresh sidebar
        loadSessions();
    };

    function showToast(sessionId, projectName) {
        const toast = document.getElementById('notification-toast');
        const countdownSpan = document.getElementById('toast-countdown');
        toast.classList.remove('hidden');
        
        let timeLeft = 5;
        countdownSpan.innerText = timeLeft;
        
        if (toastTimeout) clearInterval(toastTimeout);
        
        toastTimeout = setInterval(() => {
            timeLeft--;
            countdownSpan.innerText = timeLeft;
            if (timeLeft <= 0) {
                clearInterval(toastTimeout);
                toast.classList.add('hidden');
                loadSession(sessionId, projectName);
            }
        }, 1000);
        
        document.getElementById('toast-cancel').onclick = () => {
            clearInterval(toastTimeout);
            toast.classList.add('hidden');
        };
        
        document.getElementById('toast-go').onclick = () => {
            clearInterval(toastTimeout);
            toast.classList.add('hidden');
            loadSession(sessionId, projectName);
        };
    }
});
