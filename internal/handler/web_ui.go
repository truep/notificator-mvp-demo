package handler

const EnhancedWebUI = `<!DOCTYPE html>
<html>
<head>
    <title>Notification MVP Demo</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        .section { margin: 20px 0; padding: 20px; border: 1px solid #ddd; border-radius: 8px; background: white; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
        .messages { height: 200px; overflow-y: auto; border: 1px solid #ccc; padding: 10px; margin: 10px 0; background: #fafafa; border-radius: 4px; }
        .message { margin: 5px 0; padding: 8px; background: #fff; border-radius: 4px; border-left: 4px solid #007bff; }
        .client-list, .pending-list { height: 150px; overflow-y: auto; border: 1px solid #ccc; padding: 10px; margin: 10px 0; background: #fafafa; border-radius: 4px; }
        .client-item { margin: 5px 0; padding: 8px; background: #e8f5e8; border-radius: 4px; display: flex; justify-content: space-between; }
        .pending-item { margin: 5px 0; padding: 8px; background: #fff3cd; border-radius: 4px; }
        button { padding: 10px 15px; margin: 5px; border: none; border-radius: 4px; cursor: pointer; background: #007bff; color: white; }
        button:hover { background: #0056b3; }
        button:disabled { background: #6c757d; cursor: not-allowed; }
        button.secondary { background: #6c757d; }
        button.success { background: #28a745; }
        button.warning { background: #ffc107; color: #212529; }
        button.danger { background: #dc3545; }
        input, textarea, select { padding: 8px; margin: 5px; width: 300px; border: 1px solid #ddd; border-radius: 4px; }
        textarea { height: 60px; }
        .status-connected { color: #28a745; font-weight: bold; }
        .status-disconnected { color: #dc3545; font-weight: bold; }
        .stats { display: flex; justify-content: space-around; margin: 20px 0; }
        .stat { text-align: center; padding: 15px; background: #e9ecef; border-radius: 8px; min-width: 120px; }
        .stat-value { font-size: 24px; font-weight: bold; color: #007bff; }
        h1 { color: #333; text-align: center; margin-bottom: 10px; }
        h2 { color: #666; border-bottom: 2px solid #007bff; padding-bottom: 5px; margin-bottom: 15px; }
        h3 { color: #666; margin-bottom: 10px; }
        .target-selection { margin: 15px 0; }
        .target-input { margin: 10px 0; }
        .user-checkbox { margin: 5px 0; }
        small { color: #666; }
        .auto-refresh { margin: 10px 0; }
        .refresh-status { font-size: 12px; color: #666; }
        .notification-form { background: #f8f9fa; padding: 15px; border-radius: 8px; margin: 10px 0; }
        .form-row { margin: 10px 0; }
        .user-list { max-height: 200px; overflow-y: auto; border: 1px solid #ddd; padding: 10px; margin: 10px 0; }
        .online-indicator { display: inline-block; width: 8px; height: 8px; background: #28a745; border-radius: 50%; margin-right: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔔 Notification MVP Demo</h1>
        <p style="text-align: center; color: #666;">Real-time notification system with Redis Streams</p>
        
        <div class="stats">
            <div class="stat">
                <div class="stat-value" id="connectedCount">0</div>
                <small>Connected Clients</small>
            </div>
            <div class="stat">
                <div class="stat-value" id="pendingCount">0</div>
                <small>Pending Notifications</small>
            </div>
            <div class="stat">
                <div class="stat-value" id="usersCount">0</div>
                <small>Available Users</small>
            </div>
        </div>

        <div class="grid">
            <div class="section">
                <h2>WebSocket Connection</h2>
                <div class="form-row">
                    <input type="number" id="userId" placeholder="User ID" value="1" min="1">
                    <input type="text" id="login" placeholder="Login" value="test_user">
                </div>
                <div>
                    <button onclick="connect()">Connect</button>
                    <button onclick="disconnect()" class="secondary">Disconnect</button>
                </div>
                <div style="margin-top: 10px;">
                    Status: <span id="status" class="status-disconnected">Disconnected</span>
                </div>
            </div>

            <div class="section">
                <h2>Connected Clients</h2>
                <div class="auto-refresh">
                    <label>
                        <input type="checkbox" id="autoRefreshClients" checked onchange="toggleAutoRefresh()">
                        Auto-refresh every 3s
                    </label>
                </div>
                <div id="clientsList" class="client-list">
                    <div style="text-align: center; color: #666;">Loading...</div>
                </div>
                <button onclick="refreshClients()" class="secondary">Refresh Now</button>
                <div class="refresh-status" id="clientsRefreshStatus"></div>
            </div>
        </div>

        <div class="section">
            <h2>Send Notification</h2>
            <div class="notification-form">
                <div class="target-selection">
                    <h3>Target Selection</h3>
                    <label class="user-checkbox">
                        <input type="radio" name="targetMode" value="manual" checked onchange="toggleTargetMode()">
                        Manual Input
                    </label>
                    <label class="user-checkbox">
                        <input type="radio" name="targetMode" value="connected" onchange="toggleTargetMode()">
                        Send to All Connected Users
                    </label>
                    <label class="user-checkbox">
                        <input type="radio" name="targetMode" value="select" onchange="toggleTargetMode()">
                        Select Multiple Users
                    </label>
                </div>
                
                <div id="manualTarget" class="target-input">
                    <div class="form-row">
                        <input type="number" id="targetId" placeholder="Target ID" value="1" min="1">
                        <input type="text" id="targetLogin" placeholder="Target Login" value="test_user">
                    </div>
                </div>
                
                <div id="connectedTarget" class="target-input" style="display: none;">
                    <div class="user-list" id="connectedUsersList">
                        <div style="text-align: center; color: #666;">Will send to all connected users</div>
                    </div>
                </div>
                
                <div id="selectTarget" class="target-input" style="display: none;">
                    <div class="user-list" id="selectableUsersList">
                        <div style="text-align: center; color: #666;">Loading users...</div>
                    </div>
                </div>
                
                <div class="form-row">
                    <input type="text" id="source" placeholder="Source" value="demo-client">
                    <textarea id="message" placeholder="Message">Hello from notification demo! 👋</textarea>
                </div>
                <div>
                    <button onclick="sendNotification()" class="success">Send Notification</button>
                    <button onclick="sendMultipleNotifications()" class="warning">Send 5 Test Notifications</button>
                </div>
            </div>
        </div>

        <div class="grid">
            <div class="section">
                <h2>Received Messages</h2>
                <div id="messages" class="messages"></div>
                <div>
                    <button onclick="clearMessages()" class="warning">Clear Messages</button>
                    <label style="margin-left: 10px;">
                        <input type="checkbox" id="autoScroll" checked>
                        Auto-scroll
                    </label>
                </div>
            </div>

            <div class="section">
                <h2>Pending Notifications</h2>
                <div class="auto-refresh">
                    <label>
                        <input type="checkbox" id="autoRefreshPending" checked onchange="toggleAutoRefresh()">
                        Auto-refresh every 5s
                    </label>
                </div>
                <div id="pendingList" class="pending-list">
                    <div style="text-align: center; color: #666;">Loading...</div>
                </div>
                <button onclick="refreshPending()" class="secondary">Refresh Now</button>
                <div class="refresh-status" id="pendingRefreshStatus"></div>
            </div>
        </div>
    </div>

    <script>
        let ws = null;
        let autoRefreshIntervals = {};

        // Инициализация
        document.addEventListener('DOMContentLoaded', function() {
            refreshClients();
            refreshPending();
            loadAvailableUsers();
            startAutoRefresh();
        });

        function connect() {
            const userId = document.getElementById('userId').value;
            const login = document.getElementById('login').value;
            
            if (!userId || !login) {
                alert('Please enter User ID and Login');
                return;
            }

            const wsUrl = 'ws://' + window.location.host + '/ws?user_id=' + userId + '&login=' + login;
            ws = new WebSocket(wsUrl);
            
            ws.onopen = function() {
                document.getElementById('status').textContent = 'Connected';
                document.getElementById('status').className = 'status-connected';
                addMessage('✅ Connected to WebSocket', 'system');
                setTimeout(refreshClients, 1000); // Refresh clients after connection
            };
            
            ws.onmessage = function(event) {
                const msg = JSON.parse(event.data);
                addMessage('📨 Received: ' + JSON.stringify(msg, null, 2), 'received');
                
                // Auto-ACK notifications
                if (msg.type === 'notification.push' && msg.data.status === 'unread') {
                    setTimeout(() => {
                        const ackMsg = {
                            type: 'notification.read',
                            data: {
                                notification_id: msg.data.notification_id,
                                stream_id: msg.data.stream_id
                            }
                        };
                        ws.send(JSON.stringify(ackMsg));
                        addMessage('✅ Sent ACK for: ' + msg.data.notification_id, 'system');
                        setTimeout(refreshPending, 1000); // Refresh pending after ACK
                    }, 1000);
                }
            };
            
            ws.onclose = function() {
                document.getElementById('status').textContent = 'Disconnected';
                document.getElementById('status').className = 'status-disconnected';
                addMessage('❌ WebSocket closed', 'system');
                setTimeout(refreshClients, 1000); // Refresh clients after disconnection
            };
            
            ws.onerror = function(error) {
                addMessage('💥 WebSocket error: ' + error, 'error');
            };
        }

        function disconnect() {
            if (ws) {
                ws.close();
                ws = null;
            }
        }

        function sendNotification() {
            const targetMode = document.querySelector('input[name="targetMode"]:checked').value;
            let targets = [];
            
            if (targetMode === 'manual') {
                const targetId = parseInt(document.getElementById('targetId').value);
                const targetLogin = document.getElementById('targetLogin').value;
                if (!targetId || !targetLogin) {
                    alert('Please fill target ID and login');
                    return;
                }
                targets = [{ id: targetId, login: targetLogin }];
            } else if (targetMode === 'connected') {
                // Get connected users from API
                fetch('/api/v1/admin/users')
                    .then(response => response.json())
                    .then(data => {
                        targets = data.available_users || [];
                        if (targets.length === 0) {
                            alert('No connected users found');
                            return;
                        }
                        sendNotificationWithTargets(targets);
                    })
                    .catch(error => {
                        addMessage('❌ Error getting connected users: ' + error, 'error');
                    });
                return;
            } else if (targetMode === 'select') {
                const checkboxes = document.querySelectorAll('#selectableUsersList input[type="checkbox"]:checked');
                if (checkboxes.length === 0) {
                    alert('Please select at least one user');
                    return;
                }
                targets = Array.from(checkboxes).map(cb => {
                    return { id: parseInt(cb.dataset.userId), login: cb.dataset.login };
                });
            }
            
            sendNotificationWithTargets(targets);
        }

        function sendNotificationWithTargets(targets) {
            const source = document.getElementById('source').value;
            const message = document.getElementById('message').value;
            
            if (!source || !message) {
                alert('Please fill source and message');
                return;
            }

            const notification = {
                target: targets,
                message: message,
                source: source,
                created_at: new Date().toISOString()
            };

            fetch('/api/v1/notify', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(notification)
            })
            .then(response => response.json())
            .then(data => {
                addMessage('📤 Notification sent to ' + targets.length + ' recipient(s): ' + JSON.stringify(data, null, 2), 'sent');
                setTimeout(refreshPending, 1000); // Refresh pending after sending
            })
            .catch(error => {
                addMessage('❌ Error sending notification: ' + error, 'error');
            });
        }

        function sendMultipleNotifications() {
            for (let i = 1; i <= 5; i++) {
                setTimeout(() => {
                    const originalMessage = document.getElementById('message').value;
                    document.getElementById('message').value = originalMessage + ' #' + i;
                    sendNotification();
                    if (i === 5) {
                        // Restore original message
                        setTimeout(() => {
                            document.getElementById('message').value = originalMessage;
                        }, 100);
                    }
                }, (i - 1) * 500);
            }
        }

        function addMessage(text, type = 'info') {
            const div = document.createElement('div');
            div.className = 'message message-' + type;
            div.innerHTML = '<small>' + new Date().toLocaleTimeString() + '</small><br>' + text;
            const messagesDiv = document.getElementById('messages');
            messagesDiv.appendChild(div);
            
            if (document.getElementById('autoScroll').checked) {
                messagesDiv.scrollTop = messagesDiv.scrollHeight;
            }
        }

        function clearMessages() {
            document.getElementById('messages').innerHTML = '';
        }

        function refreshClients() {
            fetch('/api/v1/admin/clients')
                .then(response => response.json())
                .then(data => {
                    const clientsList = document.getElementById('clientsList');
                    const clients = data.connected_clients || [];
                    
                    document.getElementById('connectedCount').textContent = clients.length;
                    
                    if (clients.length === 0) {
                        clientsList.innerHTML = '<div style="text-align: center; color: #666;">No connected clients</div>';
                    } else {
                        clientsList.innerHTML = clients.map(client => 
                            '<div class="client-item">' +
                            '<span><span class="online-indicator"></span>' + client.login + ' (ID: ' + client.user_id + ')</span>' +
                            '<small>' + new Date(client.connected_at).toLocaleTimeString() + '</small>' +
                            '</div>'
                        ).join('');
                    }
                    
                    document.getElementById('clientsRefreshStatus').textContent = 'Last updated: ' + new Date().toLocaleTimeString();
                    updateConnectedUsersList(clients);
                })
                .catch(error => {
                    document.getElementById('clientsList').innerHTML = '<div style="color: #dc3545;">Error loading clients</div>';
                });
        }

        function refreshPending() {
            fetch('/api/v1/admin/pending')
                .then(response => response.json())
                .then(data => {
                    const pendingList = document.getElementById('pendingList');
                    const pending = data.pending_notifications || {};
                    const totalPending = data.total_pending || 0;
                    
                    document.getElementById('pendingCount').textContent = totalPending;
                    
                    if (totalPending === 0) {
                        pendingList.innerHTML = '<div style="text-align: center; color: #666;">No pending notifications</div>';
                    } else {
                        let html = '';
                        for (const userKey in pending) {
                            const messages = pending[userKey];
                            html += '<div style="font-weight: bold; margin-top: 10px;">' + userKey + ' (' + messages.length + ' pending)</div>';
                            messages.forEach(msg => {
                                const payload = msg.payload;
                                html += '<div class="pending-item">' +
                                    '<div>' + (payload ? payload.message : 'Expired') + '</div>' +
                                    '<small>ID: ' + (payload ? payload.notification_id.substring(0, 8) + '...' : 'N/A') + '</small>' +
                                    '</div>';
                            });
                        }
                        pendingList.innerHTML = html;
                    }
                    
                    document.getElementById('pendingRefreshStatus').textContent = 'Last updated: ' + new Date().toLocaleTimeString();
                })
                .catch(error => {
                    document.getElementById('pendingList').innerHTML = '<div style="color: #dc3545;">Error loading pending</div>';
                });
        }

        function loadAvailableUsers() {
            fetch('/api/v1/admin/users')
                .then(response => response.json())
                .then(data => {
                    const users = data.available_users || [];
                    document.getElementById('usersCount').textContent = users.length;
                    updateSelectableUsersList(users);
                })
                .catch(error => {
                    console.error('Error loading available users:', error);
                });
        }

        function updateConnectedUsersList(clients) {
            const connectedList = document.getElementById('connectedUsersList');
            if (clients.length === 0) {
                connectedList.innerHTML = '<div style="text-align: center; color: #666;">No connected users</div>';
            } else {
                connectedList.innerHTML = '<div style="color: #28a745; font-weight: bold;">Will send to:</div>' +
                    clients.map(client => 
                        '<div><span class="online-indicator"></span>' + client.login + ' (ID: ' + client.user_id + ')</div>'
                    ).join('');
            }
        }

        function updateSelectableUsersList(users) {
            const selectList = document.getElementById('selectableUsersList');
            if (users.length === 0) {
                selectList.innerHTML = '<div style="text-align: center; color: #666;">No users available</div>';
            } else {
                selectList.innerHTML = users.map(user => 
                    '<label class="user-checkbox">' +
                    '<input type="checkbox" data-user-id="' + user.id + '" data-login="' + user.login + '"> ' +
                    '<span class="online-indicator"></span>' + user.login + ' (ID: ' + user.id + ')' +
                    '</label>'
                ).join('');
            }
        }

        function toggleTargetMode() {
            const mode = document.querySelector('input[name="targetMode"]:checked').value;
            
            document.getElementById('manualTarget').style.display = mode === 'manual' ? 'block' : 'none';
            document.getElementById('connectedTarget').style.display = mode === 'connected' ? 'block' : 'none';
            document.getElementById('selectTarget').style.display = mode === 'select' ? 'block' : 'none';
            
            if (mode === 'connected') {
                refreshClients();
            } else if (mode === 'select') {
                loadAvailableUsers();
            }
        }

        function toggleAutoRefresh() {
            startAutoRefresh();
        }

        function startAutoRefresh() {
            // Clear existing intervals
            Object.values(autoRefreshIntervals).forEach(interval => clearInterval(interval));
            autoRefreshIntervals = {};
            
            // Start new intervals if enabled
            if (document.getElementById('autoRefreshClients').checked) {
                autoRefreshIntervals.clients = setInterval(refreshClients, 3000);
            }
            
            if (document.getElementById('autoRefreshPending').checked) {
                autoRefreshIntervals.pending = setInterval(refreshPending, 5000);
            }
        }

        // Cleanup on page unload
        window.addEventListener('beforeunload', function() {
            Object.values(autoRefreshIntervals).forEach(interval => clearInterval(interval));
            if (ws) {
                ws.close();
            }
        });
    </script>
</body>
</html>`
