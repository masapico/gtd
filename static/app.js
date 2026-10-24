// ==========================================================================
// GTD Desktop Companion Client Script
// ==========================================================================

// 1. クリップボードからの画像ペースト (Ctrl+V / Cmd+V)
let isUploadingImage = false;

document.addEventListener('paste', async (e) => {
  const detailPane = document.getElementById('item-detail-pane');
  if (!detailPane || detailPane.classList.contains('closed')) {
    return;
  }

  // Quick Capture にフォーカスがある場合はタスク詳細への画像貼り付けを行わない
  if (document.activeElement && document.activeElement.id === 'quick-capture-input') {
    return;
  }

  // item-detail-pane または内部の item-detail-content から itemId を取得
  const contentEl = document.getElementById('item-detail-content') || detailPane.querySelector('[data-item-id]');
  const itemId = detailPane.dataset.itemId || contentEl?.dataset?.itemId;
  if (!itemId) {
    return;
  }

  // 入力フォームにフォーカスがあり、テキストがクリップボードにある場合は通常のテキストペーストを優先
  const activeTag = document.activeElement ? document.activeElement.tagName.toLowerCase() : '';
  const isInputActive = activeTag === 'input' || activeTag === 'textarea';
  const hasText = e.clipboardData?.types?.includes('text/plain') && e.clipboardData.getData('text/plain').length > 0;
  if (isInputActive && hasText) {
    return;
  }

  // 画像データの抽出 (files または items から)
  let imageFile = null;
  if (e.clipboardData?.files && e.clipboardData.files.length > 0) {
    for (const f of e.clipboardData.files) {
      if (f.type && f.type.startsWith('image/')) {
        imageFile = f;
        break;
      }
    }
  }
  if (!imageFile && e.clipboardData?.items) {
    for (const item of e.clipboardData.items) {
      if (item.type && item.type.startsWith('image/')) {
        imageFile = item.getAsFile();
        if (imageFile) break;
      }
    }
  }

  if (!imageFile) return;

  // 画像ペースト処理の実行
  e.preventDefault();
  if (isUploadingImage) return;
  isUploadingImage = true;

  const formData = new FormData();
  formData.append('image', imageFile, imageFile.name || 'screenshot.png');

  try {
    showToast('スクリーンショットをアップロード中...');
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
      // 中央リストの添付数バッジやサイドバー等を最新化
      if (window.htmx) {
        htmx.trigger(document.body, 'itemChanged');
      }
    } else {
      const errText = await res.text().catch(() => '');
      showToast('画像保存に失敗しました' + (errText ? `: ${errText}` : ''));
    }
  } catch (err) {
    console.error('Failed to paste image', err);
    showToast('画像保存エラー');
  } finally {
    isUploadingImage = false;
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
function openDetailPane(itemId) {
  const detailPane = document.getElementById('item-detail-pane');
  if (detailPane) {
    detailPane.classList.remove('closed');
    if (itemId) {
      detailPane.dataset.itemId = itemId;
    }
  }
}

function closeDetailPane() {
  const detailPane = document.getElementById('item-detail-pane');
  if (detailPane) {
    detailPane.classList.add('closed');
    delete detailPane.dataset.itemId;
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
function showToast(message, isError) {
  if (!message) return;
  message = decodeToastMessage(message);

  let toast = document.getElementById('app-toast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'app-toast';
    document.body.appendChild(toast);
  }
  toast.className = 'toast-notice' + (isError ? ' toast-error' : '');
  toast.innerText = message;
  toast.style.opacity = '1';
  clearTimeout(toast._timeout);
  const duration = (message && message.length > 25) || isError ? 5000 : 2500;
  toast._timeout = setTimeout(() => {
    toast.style.opacity = '0';
  }, duration);
}

// トーストメッセージのデコード（URLエンコード解除および Latin-1 文字化けの自動修復）
function decodeToastMessage(raw) {
  if (!raw) return '';
  let str = String(raw);
  // 1. URLエンコード (%E3%83...) されている場合はデコード
  try {
    if (str.includes('%')) {
      str = decodeURIComponent(str.replace(/\+/g, ' '));
    }
  } catch (_) {}

  // 2. HTTPヘッダー経由で UTF-8 が Latin-1 (ISO-8859-1) として解釈された文字化け（ã... など）を修復
  if (/[\u0080-\u00FF]/.test(str)) {
    try {
      const bytes = Uint8Array.from(str, c => c.charCodeAt(0));
      const decoded = new TextDecoder('utf-8', { fatal: true }).decode(bytes);
      if (decoded) str = decoded;
    } catch (_) {}
  }
  return str;
}

// 添付ファイルオープン成功時のトースト通知
document.addEventListener('attachmentOpened', () => {
  showToast('ファイルまたはリンクを開きました');
});

// HTMX のカスタムトリガー 'showToast' をリッスン
document.addEventListener('showToast', (e) => {
  const rawMsg = typeof e.detail === 'string' ? e.detail : (e.detail?.value || '完了しました');
  showToast(rawMsg);
});

// HTMX リクエスト失敗時のエラー通知 (HTMX 4: htmx:response:error, HTMX 1/2: htmx:responseError)
function handleHtmxError(e) {
  const ctx = e.detail?.ctx;
  const xhr = e.detail?.xhr;
  const status = ctx?.response?.status || xhr?.status || 0;
  let msg = ctx?.text || xhr?.responseText || '';
  if (!msg && e.detail?.error) {
    msg = e.detail.error.message || String(e.detail.error);
  }
  if (!msg) {
    msg = status ? `リクエストに失敗しました (${status})` : 'エラーが発生しました';
  }
  showToast(msg.trim(), true);
}

document.addEventListener('htmx:response:error', handleHtmxError);
document.addEventListener('htmx:responseError', handleHtmxError);
document.addEventListener('htmx:error', handleHtmxError);

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

