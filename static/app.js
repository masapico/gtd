// ==========================================================================
// GTD Desktop Companion Client Script
// ==========================================================================

// 1. クリップボードからの画像ペースト (Ctrl+V / Cmd+V)
document.addEventListener('paste', async (e) => {
  const detailPane = document.getElementById('item-detail-pane');
  if (!detailPane || detailPane.classList.contains('closed') || !detailPane.dataset.itemId) {
    return;
  }

  const items = e.clipboardData?.items;
  if (!items) return;

  for (const item of items) {
    if (item.type.indexOf('image') !== -1) {
      e.preventDefault();
      const file = item.getAsFile();
      const formData = new FormData();
      formData.append('image', file);

      const itemId = detailPane.dataset.itemId;
      try {
        const res = await fetch(`/items/${itemId}/paste-image`, {
          method: 'POST',
          body: formData,
        });
        if (res.ok) {
          showToast('スクリーンショットを保存しました');
          // 添付一覧を再取得
          const attContainer = document.getElementById('attachments-container');
          if (attContainer && window.htmx) {
            htmx.ajax('GET', `/items/${itemId}/attachments`, { target: '#attachments-container', swap: 'innerHTML' });
          }
        } else {
          showToast('画像保存に失敗しました');
        }
      } catch (err) {
        console.error('Failed to paste image', err);
        showToast('画像保存エラー');
      }
      break;
    }
  }
});

// 2. キーボードショートカット
document.addEventListener('keydown', (e) => {
  // Esc: プロジェクトインライン編集を閉じる / 詳細インスペクタを閉じる / サイドバーを閉じる
  if (e.key === 'Escape') {
    const projectEditForm = document.getElementById('project-title-edit');
    if (projectEditForm && projectEditForm.style.display !== 'none') {
      e.preventDefault();
      toggleProjectEdit(false);
      return;
    }
    // プロジェクト一覧内のカード編集フォームも閉じる
    const openCardEdits = document.querySelectorAll('form[id^="project-card-edit-"]');
    for (const form of openCardEdits) {
      if (form.style.display !== 'none') {
        e.preventDefault();
        const id = form.id.replace('project-card-edit-', '');
        toggleProjectCardEdit(id, false);
        return;
      }
    }

    closeDetailPane();
    const sidebar = document.getElementById('sidebar-pane');
    if (sidebar) sidebar.classList.remove('open');
    return;
  }

  // フォーカスが入力フォーム中ならショートカットを発火しない
  const activeTag = document.activeElement ? document.activeElement.tagName.toLowerCase() : '';
  const isInputActive = activeTag === 'input' || activeTag === 'textarea' || activeTag === 'select';

  // Alt + N: Quick Capture 入力欄へフォーカス
  if (e.altKey && (e.key === 'n' || e.key === 'N')) {
    e.preventDefault();
    focusQuickCapture();
    return;
  }

  // 入力フォーム外での '/' キー: Quick Capture 入力欄へフォーカス
  if (!isInputActive && e.key === '/') {
    e.preventDefault();
    focusQuickCapture();
    return;
  }
});

function focusQuickCapture() {
  const input = document.getElementById('quick-capture-input');
  if (input) {
    input.focus();
    input.select();
  }
}

// 3. 詳細ペインの開閉制御
function openDetailPane() {
  const detailPane = document.getElementById('item-detail-pane');
  if (detailPane) {
    detailPane.classList.remove('closed');
  }
}

function closeDetailPane() {
  const detailPane = document.getElementById('item-detail-pane');
  if (detailPane) {
    detailPane.classList.add('closed');
  }
  // リストの選択ハイライト解除
  document.querySelectorAll('.item-card.selected').forEach(el => el.classList.remove('selected'));
}

// 4. サイドバーの開閉制御（小ウィンドウ用）
function toggleSidebar() {
  const sidebar = document.getElementById('sidebar-pane');
  if (sidebar) {
    sidebar.classList.toggle('open');
  }
}

// 5. ローカルパスのクリップボードコピー
async function copyToClipboard(text, btnElement) {
  try {
    await navigator.clipboard.writeText(text);
    showToast('パスをクリップボードにコピーしました');
    if (btnElement) {
      const originalText = btnElement.innerText;
      btnElement.innerText = 'コピー完了';
      setTimeout(() => {
        btnElement.innerText = originalText;
      }, 1500);
    }
  } catch (err) {
    console.error('Failed to copy', err);
    showToast('コピーに失敗しました');
  }
}

// 6. トーストメッセージ表示
function showToast(message) {
  let toast = document.getElementById('app-toast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'app-toast';
    toast.className = 'toast-notice';
    document.body.appendChild(toast);
  }
  toast.innerText = message;
  toast.style.opacity = '1';
  clearTimeout(toast._timeout);
  toast._timeout = setTimeout(() => {
    toast.style.opacity = '0';
  }, 2500);
}

// 7. 中央ペインの hx-get 属性を動的に追従（ビュー切り替え後のリフレッシュ先を維持）
function syncMainPaneUrl(e) {
  const target = e.detail?.target;
  if (target && target.id === 'main-content-pane') {
    const cfg = e.detail.requestConfig;
    const method = (cfg?.verb || cfg?.method || '').toLowerCase();
    const path = cfg?.path;
    if (method === 'get' && path && path.startsWith('/partials/items')) {
      target.setAttribute('hx-get', path);
    }
  }
}
document.addEventListener('htmx:afterRequest', syncMainPaneUrl);
document.addEventListener('htmx:after:request', syncMainPaneUrl);

// 8. プロジェクト名・詳細のインライン編集制御
function toggleProjectEdit(show) {
  const viewEl = document.getElementById('project-title-view');
  const editEl = document.getElementById('project-title-edit');
  if (!viewEl || !editEl) return;

  if (show) {
    viewEl.style.display = 'none';
    editEl.style.display = 'block';
    const input = document.getElementById('project-edit-name-input');
    if (input) {
      input.focus();
      input.select();
    }
  } else {
    viewEl.style.display = 'block';
    editEl.style.display = 'none';
  }
}

function toggleProjectCardEdit(id, show) {
  const viewEl = document.getElementById('project-card-view-' + id);
  const editEl = document.getElementById('project-card-edit-' + id);
  if (!viewEl || !editEl) return;

  if (show) {
    viewEl.style.display = 'none';
    editEl.style.display = 'block';
    const input = document.getElementById('project-list-edit-input-' + id);
    if (input) {
      input.focus();
      input.select();
    }
  } else {
    viewEl.style.display = 'block';
    editEl.style.display = 'none';
  }
}

