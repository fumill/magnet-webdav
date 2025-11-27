let currentMagnetId = null;

// 加载统计数据
async function loadStats() {
    try {
        const response = await fetch('/api/stats');
        const stats = await response.json();

        document.getElementById('stats').innerHTML = `
            <div class="stat-item">
                <div class="stat-value">${stats.total_magnets}</div>
                <div>总磁力链接</div>
            </div>
            <div class="stat-item">
                <div class="stat-value">${stats.total_files}</div>
                <div>总文件数</div>
            </div>
            <div class="stat-item">
                <div class="stat-value">${stats.active_torrents}</div>
                <div>活跃种子</div>
            </div>
        `;
    } catch (error) {
        console.error('Failed to load stats:', error);
    }
}

// 添加磁力链接
async function addMagnet() {
    const magnetInput = document.getElementById('magnetInput');
    const magnetUri = magnetInput.value.trim();

    if (!magnetUri) {
        alert('请输入磁力链接');
        return;
    }

    try {
        const response = await fetch('/api/magnets', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({ magnet_uri: magnetUri }),
        });

        if (response.ok) {
            magnetInput.value = '';
            alert('磁力链接添加成功！');
            loadMagnets();
            loadStats();
        } else {
            const error = await response.json();
            alert('添加失败: ' + error.error);
        }
    } catch (error) {
        alert('网络错误: ' + error.message);
    }
}

// 加载磁力链接列表
async function loadMagnets() {
    const list = document.getElementById('magnetsList');
    list.innerHTML = '<div class="loading">加载中...</div>';

    try {
        const response = await fetch('/api/magnets');
        const magnets = await response.json();

        if (magnets.length === 0) {
            list.innerHTML = '<div class="loading">暂无磁力链接</div>';
            return;
        }

        list.innerHTML = magnets.map(magnet => `
            <div class="magnet-item">
                <div class="magnet-header">
                    <div class="magnet-name">${magnet.name || magnet.id}</div>
                    <div class="magnet-status ${magnet.status}">${getStatusText(magnet.status)}</div>
                </div>
                <div class="magnet-info">
                    <span>文件数: ${magnet.file_count || 0}</span>
                    <span>大小: ${formatFileSize(magnet.total_size || 0)}</span>
                    <span>访问: ${magnet.access_count} 次</span>
                    <span>添加: ${new Date(magnet.created_at).toLocaleDateString()}</span>
                </div>
                <div class="magnet-actions">
                    <button class="btn btn-primary" onclick="viewFiles('${magnet.id}')">查看文件</button>
                    <a href="/webdav/${magnet.id}" class="btn btn-primary" target="_blank">WebDAV 访问</a>
                    <button class="btn btn-danger" onclick="removeMagnet('${magnet.id}')">删除</button>
                </div>
            </div>
        `).join('');
    } catch (error) {
        list.innerHTML = '<div class="error">加载失败: ' + error.message + '</div>';
    }
}

// 查看文件列表
async function viewFiles(magnetId) {
    currentMagnetId = magnetId;
    const section = document.getElementById('filesSection');
    const list = document.getElementById('filesList');

    section.style.display = 'block';
    list.innerHTML = '<div class="loading">加载中...</div>';

    try {
        const response = await fetch(`/api/magnets/${magnetId}/files`);
        const files = await response.json();

        if (files.length === 0) {
            list.innerHTML = '<div class="loading">暂无文件</div>';
            return;
        }

        list.innerHTML = files.map(file => `
            <div class="file-item">
                <div class="file-info">
                    <div class="file-name">${file.file_name}</div>
                    <div class="file-details">
                        ${formatFileSize(file.file_size)} • ${file.mime_type}
                        ${file.duration ? `• 时长: ${formatDuration(file.duration)}` : ''}
                    </div>
                </div>
                <div class="file-actions">
                    <a href="/webdav/${magnetId}/${encodeURIComponent(file.file_path)}" 
                       class="btn btn-primary" target="_blank">播放</a>
                </div>
            </div>
        `).join('');
    } catch (error) {
        list.innerHTML = '<div class="error">加载失败: ' + error.message + '</div>';
    }
}

// 删除磁力链接
async function removeMagnet(magnetId) {
    if (!confirm('确定要删除这个磁力链接吗？')) {
        return;
    }

    try {
        const response = await fetch(`/api/magnets/${magnetId}`, {
            method: 'DELETE',
        });

        if (response.ok) {
            alert('删除成功！');
            loadMagnets();
            loadStats();

            // 如果删除的是当前查看的文件，隐藏文件列表
            if (currentMagnetId === magnetId) {
                document.getElementById('filesSection').style.display = 'none';
                currentMagnetId = null;
            }
        } else {
            const error = await response.json();
            alert('删除失败: ' + error.error);
        }
    } catch (error) {
        alert('网络错误: ' + error.message);
    }
}

// 工具函数
function getStatusText(status) {
    const statusMap = {
        'pending': '等待中',
        'metadata': '获取元数据',
        'ready': '就绪',
        'error': '错误'
    };
    return statusMap[status] || status;
}

function formatFileSize(bytes) {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDuration(seconds) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;

    if (hours > 0) {
        return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    } else {
        return `${minutes}:${secs.toString().padStart(2, '0')}`;
    }
}

// 初始化
document.addEventListener('DOMContentLoaded', function() {
    loadStats();
    loadMagnets();

    // 支持回车键添加磁力链接
    document.getElementById('magnetInput').addEventListener('keypress', function(e) {
        if (e.key === 'Enter') {
            addMagnet();
        }
    });
});
