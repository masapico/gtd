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
  // Ctrl + I / Cmd + I: Quick Capture 入力欄へフォーカス
  if ((e.ctrlKey || e.metaKey) && (e.key === 'i' || e.key === 'I')) {
    e.preventDefault();
    focusQuickCapture();
    return;
  }

  // Ctrl + F / Cmd + F / Ctrl + K / Cmd + K: Omnisearch 検索窓へフォーカス
  if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.key === 'F' || e.key === 'k' || e.key === 'K')) {
    e.preventDefault();
    focusOmnisearch();
    return;
  }

  // Ctrl + L / Cmd + L: アプリケーションランチャーモーダルを開く
  if ((e.ctrlKey || e.metaKey) && (e.key === 'l' || e.key === 'L')) {
    e.preventDefault();
    openLauncherModal();
    return;
  }

  const dropdown = document.getElementById('omnisearch-dropdown');
  const isDropdownOpen = dropdown && !dropdown.classList.contains('hidden') && dropdown.innerHTML.trim() !== '';
  const searchInput = document.getElementById('omnisearch-input');
  const isSearchFocused = document.activeElement === searchInput;

  // Esc: モーダルを閉じる / オムニサーチドロップダウンを閉じる / プロジェクトインライン編集を閉じる / 詳細インスペクタを閉じる / サイドバーを閉じる
  if (e.key === 'Escape') {
    const shortcutsModal = document.getElementById('shortcuts-modal');
    if (shortcutsModal && !shortcutsModal.classList.contains('hidden')) {
      e.preventDefault();
      closeShortcutsHelp();
      return;
    }

    // サイドバーのインライン追加フォームを閉じる
    const openAddForm = document.activeElement?.closest?.('.sidebar-add-form');
    if (openAddForm) {
      e.preventDefault();
      toggleSidebarAddForm(openAddForm.id, false);
      return;
    }

    const clockModal = document.getElementById('clock-modal');
    if (clockModal && !clockModal.classList.contains('hidden')) {
      e.preventDefault();
      closeClockModal();
      return;
    }

    const modal = document.getElementById('modal-container');
    if (modal && !modal.classList.contains('hidden') && modal.innerHTML.trim() !== '') {
      e.preventDefault();
      closeModal();
      return;
    }

    if (isDropdownOpen) {
      e.preventDefault();
      closeOmnisearchDropdown();
      if (searchInput) searchInput.blur();
      return;
    }

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

  // ドロップダウンが開いている場合の矢印キー・Enterナビゲーション
  if (isDropdownOpen && (isSearchFocused || dropdown.contains(document.activeElement))) {
    const rows = Array.from(dropdown.querySelectorAll('.search-result-row'));
    if (rows.length > 0) {
      const activeIdx = rows.findIndex(r => r.classList.contains('active'));

      if (e.key === 'ArrowDown') {
        e.preventDefault();
        let nextIdx = activeIdx + 1;
        if (nextIdx >= rows.length) nextIdx = 0;
        rows.forEach(r => r.classList.remove('active'));
        rows[nextIdx].classList.add('active');
        rows[nextIdx].scrollIntoView({ block: 'nearest' });
        return;
      }

      if (e.key === 'ArrowUp') {
        e.preventDefault();
        let prevIdx = activeIdx - 1;
        if (prevIdx < 0) prevIdx = rows.length - 1;
        rows.forEach(r => r.classList.remove('active'));
        rows[prevIdx].classList.add('active');
        rows[prevIdx].scrollIntoView({ block: 'nearest' });
        return;
      }

      if (e.key === 'Enter') {
        e.preventDefault();
        const targetRow = activeIdx >= 0 ? rows[activeIdx] : rows[0];
        if (targetRow) {
          targetRow.click();
        }
        return;
      }
    }
  }

  // フォーカスが入力フォーム中ならショートカットを発火しない
  const activeTag = document.activeElement ? document.activeElement.tagName.toLowerCase() : '';
  const isInputActive = activeTag === 'input' || activeTag === 'textarea' || activeTag === 'select';

  // 入力フォーム外での '/' キー: Quick Capture 入力欄へフォーカス
  if (!isInputActive && e.key === '/') {
    e.preventDefault();
    focusQuickCapture();
    return;
  }

  // Alt + ↑ / ↓: 選択中のカードをプロジェクト内（同じセクション内）で並び替え
  if (e.altKey && !e.ctrlKey && !e.metaKey && !e.shiftKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
    if (isInputActive || isAnyModalOpen()) return;
    if (moveSelectedCard(e.key === 'ArrowUp' ? 'up' : 'down')) e.preventDefault();
    return;
  }

  // 中央リストの単キー操作（修飾キー付き・入力中・モーダル表示中は無効）
  if (isInputActive || e.ctrlKey || e.metaKey || e.altKey || isAnyModalOpen()) return;
  if (handleListShortcut(e.key)) e.preventDefault();
});

function isAnyModalOpen() {
  return [...document.querySelectorAll('.modal-overlay')].some(m => !m.classList.contains('hidden'));
}

// 2-2. リスト操作ショートカット（j/k 移動、Enter/o 詳細、x 完了、1-5 ステータス、? ヘルプ）
const SHORTCUT_STATUSES = { '1': 'inbox', '2': 'next', '3': 'waiting', '4': 'someday', '5': 'reference' };
let selectedItemId = null;

// 完了・移動で選択中のカードが一覧から消えた場合に、代わりに選択するカード（直後、なければ直前）。
// 無関係な画面遷移で誤って使われないよう、操作直後の短時間だけ有効にする。
const SELECTION_HANDOFF_MS = 3000;
let selectionHandoff = null; // { id, at }

function rememberSelectionHandoff(cards, index) {
  const neighbor = cards[index + 1] || cards[index - 1];
  const id = neighbor ? cardItemId(neighbor) : null;
  selectionHandoff = id ? { id, at: Date.now() } : null;
}

// g に続けて押すキー → サイドバーのリンク先
const GOTO_VIEWS = {
  i: '/?view=inbox', n: '/?view=next', s: '/?view=scheduled', w: '/?view=waiting',
  m: '/?view=someday', f: '/?view=reference', d: '/?view=done',
  r: '/?view=review&step=1', p: '/?view=projects',
};
const GOTO_PREFIX_MS = 1500;
let gotoPrefixAt = 0;

function goToView(key) {
  const href = GOTO_VIEWS[key];
  const link = href && document.querySelector(`#sidebar-pane a[href="${href}"]`);
  if (!link) return false;
  link.click();
  return true;
}

function visibleItemCards() {
  return [...document.querySelectorAll('#main-content-pane .item-card')]
    .filter(el => el.offsetParent !== null && !el.classList.contains('pending-delete'));
}

function cardItemId(card) {
  const m = (card.getAttribute('hx-get') || '').match(/^\/items\/(\d+)\/detail$/);
  return m ? m[1] : null;
}

// sticky な .content-header に隠れないよう、カードを中央ペインの可視範囲へスクロールする
// （scrollIntoView の 'nearest' は sticky ヘッダーの高さを考慮しないため自前で計算する）
function scrollCardIntoView(card) {
  const pane = card.closest('#main-content-pane');
  if (!pane) {
    card.scrollIntoView({ block: 'nearest' });
    return;
  }
  // 先頭カードならペイン最上部まで戻し、プロジェクト情報などのヘッダーを全部見せる
  if (visibleItemCards()[0] === card) {
    pane.scrollTop = 0;
    return;
  }
  const header = pane.querySelector('.content-header');
  const headerH = header ? header.getBoundingClientRect().height : 0;
  const paneRect = pane.getBoundingClientRect();
  const rect = card.getBoundingClientRect();
  const margin = 8;
  const visibleTop = paneRect.top + headerH + margin;
  const visibleBottom = paneRect.bottom - margin;
  if (rect.top < visibleTop) {
    pane.scrollTop -= visibleTop - rect.top;
  } else if (rect.bottom > visibleBottom) {
    pane.scrollTop += Math.min(rect.bottom - visibleBottom, rect.top - visibleTop);
  }
}

function selectItemCard(card) {
  document.querySelectorAll('.item-card.selected').forEach(c => c.classList.remove('selected'));
  if (!card) return;
  card.classList.add('selected');
  scrollCardIntoView(card);
  selectedItemId = cardItemId(card);
}

function handleListShortcut(key) {
  if (key === '?') {
    openShortcutsHelp();
    return true;
  }

  // g → i / n / s ... でビュー移動
  const gotoPending = Date.now() - gotoPrefixAt < GOTO_PREFIX_MS;
  gotoPrefixAt = 0;
  if (gotoPending && goToView(key)) return true;
  if (key === 'g') {
    gotoPrefixAt = Date.now();
    return true;
  }

  const cards = visibleItemCards();
  if (cards.length === 0) return false;
  const current = cards.findIndex(c => c.classList.contains('selected'));
  const card = cards[current];

  if (key === 'j' || key === 'k') {
    selectionHandoff = null;
    const next = current === -1 ? 0 : Math.min(Math.max(current + (key === 'j' ? 1 : -1), 0), cards.length - 1);
    selectItemCard(cards[next]);
    return true;
  }
  if (!card) return false;

  const id = cardItemId(card);
  if (key === 'Enter' || key === 'o') {
    card.click();
    return true;
  }
  if (key === 'x') {
    const checkbox = card.querySelector('.item-checkbox');
    if (checkbox) {
      rememberSelectionHandoff(cards, current);
      checkbox.click();
    }
    return true;
  }
  if (SHORTCUT_STATUSES[key] && id) {
    rememberSelectionHandoff(cards, current);
    sendItemRequest('PATCH', `/items/${id}/status`, { status: SHORTCUT_STATUSES[key] })
      .then(() => htmx.trigger(document.body, 'itemDetailRefresh'))
      .catch(err => showToast(err.message, true));
    return true;
  }
  return false;
}

// 2-3. プロジェクト内の並び替え（ドラッグ＆ドロップ / Alt+↑↓）
// 並び替えは [data-reorder-project] を持つセクション内に限る（セクションをまたぐ移動はステータス変更で行う）
let scrollSelectedAfterSwap = false;

function reorderableCards(container) {
  return [...container.children].filter(el => el.classList.contains('item-card') && el.querySelector('.drag-handle'));
}

function moveSelectedCard(dir) {
  const card = document.querySelector('#main-content-pane [data-reorder-project] .item-card.selected');
  if (!card || !card.querySelector('.drag-handle')) return false;
  const id = cardItemId(card);
  if (!id) return false;

  // 応答を待たずに見た目を先に入れ替える（確定した並びは itemChanged の再描画で反映される）
  const siblings = reorderableCards(card.parentElement);
  const idx = siblings.indexOf(card);
  const neighbor = siblings[dir === 'up' ? idx - 1 : idx + 1];
  if (!neighbor) return true; // セクションの端
  neighbor.insertAdjacentElement(dir === 'up' ? 'beforebegin' : 'afterend', card);
  scrollCardIntoView(card);

  selectedItemId = id;
  scrollSelectedAfterSwap = true;
  sendItemRequest('POST', `/items/${id}/move`, { dir }).catch(err => {
    showToast(err.message, true);
    htmx.trigger(document.body, 'itemChanged');
  });
  return true;
}

let dragState = null; // { card, container }

function clearDropIndicators() {
  document.querySelectorAll('.item-card.drop-before, .item-card.drop-after')
    .forEach(el => el.classList.remove('drop-before', 'drop-after'));
}

// ハンドルを掴んだときだけカードをドラッグ可能にする（カードのクリックや文字選択を妨げないため）
document.addEventListener('mousedown', (e) => {
  const handle = e.target.closest?.('[data-reorder-project] .drag-handle');
  if (!handle) return;
  const card = handle.closest('.item-card');
  if (card) card.draggable = true;
});
document.addEventListener('mouseup', () => {
  if (dragState) return;
  document.querySelectorAll('[data-reorder-project] .item-card[draggable="true"]').forEach(c => { c.draggable = false; });
});

document.addEventListener('dragstart', (e) => {
  const card = e.target.closest?.('[data-reorder-project] .item-card');
  if (!card || !card.draggable) return;
  dragState = { card, container: card.parentElement };
  e.dataTransfer.effectAllowed = 'move';
  e.dataTransfer.setData('text/plain', cardItemId(card) || '');
  requestAnimationFrame(() => card.classList.add('dragging'));
});

// ドロップ位置: ポインタ直下（なければ最も近い）カードの上半分なら前、下半分なら後ろ
function dropTarget(container, clientY) {
  const cards = reorderableCards(container).filter(c => c !== dragState.card);
  if (cards.length === 0) return null;
  for (const c of cards) {
    const r = c.getBoundingClientRect();
    if (clientY < r.top + r.height / 2) return { card: c, before: true };
  }
  return { card: cards[cards.length - 1], before: false };
}

document.addEventListener('dragover', (e) => {
  if (!dragState) return;
  const container = e.target.closest?.('[data-reorder-project]');
  if (container !== dragState.container) {
    clearDropIndicators();
    return; // 別セクションにはドロップさせない
  }
  e.preventDefault();
  e.dataTransfer.dropEffect = 'move';
  const target = dropTarget(container, e.clientY);
  clearDropIndicators();
  if (target) target.card.classList.add(target.before ? 'drop-before' : 'drop-after');
});

document.addEventListener('drop', (e) => {
  if (!dragState) return;
  const container = e.target.closest?.('[data-reorder-project]');
  if (container !== dragState.container) return;
  e.preventDefault();

  const { card } = dragState;
  const target = dropTarget(container, e.clientY);
  clearDropIndicators();
  if (!target) return;

  const before = reorderableCards(container).map(cardItemId).join(',');
  target.card.insertAdjacentElement(target.before ? 'beforebegin' : 'afterend', card);
  const ids = reorderableCards(container).map(cardItemId).filter(Boolean);
  if (ids.join(',') === before) return; // 位置が変わっていない

  const projectId = container.dataset.reorderProject;
  sendItemRequest('POST', `/projects/${projectId}/reorder`, { ids: ids.join(',') }).catch(err => {
    showToast(err.message, true);
    htmx.trigger(document.body, 'itemChanged');
  });
});

document.addEventListener('dragend', () => {
  if (!dragState) return;
  dragState.card.classList.remove('dragging');
  dragState.card.draggable = false;
  dragState = null;
  clearDropIndicators();
});

// リスト再描画後も、選択していたアイテムのハイライトを復元する
function restoreSelectedCard() {
  if (!selectedItemId) return;
  const cards = visibleItemCards();
  const card = cards.find(c => cardItemId(c) === selectedItemId);
  if (card) {
    card.classList.add('selected');
    if (scrollSelectedAfterSwap) scrollCardIntoView(card);
    scrollSelectedAfterSwap = false;
    return;
  }
  // 選択していたカードが消えた: 直前のキー操作によるものなら隣のカードへ選択を移す
  const handoff = selectionHandoff;
  if (!handoff || Date.now() - handoff.at > SELECTION_HANDOFF_MS) return;
  const neighbor = cards.find(c => cardItemId(c) === handoff.id);
  if (neighbor) {
    selectionHandoff = null;
    selectItemCard(neighbor);
  }
}
document.addEventListener('htmx:afterSwap', restoreSelectedCard);
document.addEventListener('htmx:after:swap', restoreSelectedCard);
document.addEventListener('click', (e) => {
  const card = e.target.closest?.('#main-content-pane .item-card');
  if (!card) return;
  selectedItemId = cardItemId(card);
  // マウス操作（e.detail > 0）で選び直したら引き継ぎ予約は破棄。キー操作が発火させた click は対象外
  if (e.detail > 0) selectionHandoff = null;
});

// 表示中と同じ URL を履歴に積まない（同じビューの再クリックや再取得で「戻る」が空振りしないように）
document.addEventListener('htmx:before:history:update', (e) => {
  if (e.detail?.history?.path === location.pathname + location.search) e.preventDefault();
});

function openShortcutsHelp() {
  const modal = document.getElementById('shortcuts-modal');
  if (modal) modal.classList.remove('hidden');
}

function closeShortcutsHelp() {
  const modal = document.getElementById('shortcuts-modal');
  if (modal) modal.classList.add('hidden');
}

function focusQuickCapture() {
  const input = document.getElementById('quick-capture-input');
  if (input) {
    input.focus();
    input.select();
  }
}

function focusOmnisearch() {
  const input = document.getElementById('omnisearch-input');
  if (input) {
    input.focus();
    input.select();
    openOmnisearchDropdown();
  }
}

// オムニサーチの開閉・クリア・選択制御
function openOmnisearchDropdown() {
  const dropdown = document.getElementById('omnisearch-dropdown');
  const input = document.getElementById('omnisearch-input');
  if (dropdown && input && input.value.trim().length > 0 && dropdown.innerHTML.trim().length > 0) {
    dropdown.classList.remove('hidden');
  }
}

function closeOmnisearchDropdown() {
  const dropdown = document.getElementById('omnisearch-dropdown');
  if (dropdown) {
    dropdown.classList.add('hidden');
    dropdown.querySelectorAll('.search-result-row.active').forEach(r => r.classList.remove('active'));
  }
}

function clearOmnisearch() {
  const input = document.getElementById('omnisearch-input');
  const clearBtn = document.getElementById('omnisearch-clear-btn');
  const dropdown = document.getElementById('omnisearch-dropdown');
  if (input) {
    input.value = '';
    input.focus();
  }
  if (clearBtn) {
    clearBtn.classList.add('hidden');
  }
  if (dropdown) {
    dropdown.innerHTML = '';
    dropdown.classList.add('hidden');
  }
}

function selectSearchResult(itemId) {
  openDetailPane(itemId);
  closeOmnisearchDropdown();
  
  // 中央リストの選択状態を更新
  selectedItemId = String(itemId);
  selectionHandoff = null;
  document.querySelectorAll('.item-card').forEach(el => {
    if (cardItemId(el) === selectedItemId) {
      el.classList.add('selected', 'pulse-highlight');
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      setTimeout(() => el.classList.remove('pulse-highlight'), 1500);
    } else {
      el.classList.remove('selected', 'pulse-highlight');
    }
  });
}

// 検索入力時のクリアボタン表示制御
document.addEventListener('input', (e) => {
  if (e.target && e.target.id === 'omnisearch-input') {
    const clearBtn = document.getElementById('omnisearch-clear-btn');
    const val = e.target.value.trim();
    if (clearBtn) {
      if (val.length > 0) {
        clearBtn.classList.remove('hidden');
      } else {
        clearBtn.classList.add('hidden');
        closeOmnisearchDropdown();
      }
    }
  }
});

// 外側クリックでドロップダウンを閉じる
document.addEventListener('click', (e) => {
  const box = document.querySelector('.omnisearch-box');
  if (box && !box.contains(e.target)) {
    closeOmnisearchDropdown();
  }
});

// HTMX のスワップ後にドロップダウンの表示を制御 (HTMX 4: after:swap, HTMX 1/2: afterSwap)
function handleOmnisearchSwap(e) {
  const target = e.detail?.target || e.target;
  if (target && target.id === 'omnisearch-dropdown') {
    const input = document.getElementById('omnisearch-input');
    const val = input ? input.value.trim() : '';
    const hasContent = target.innerHTML.trim().length > 0;
    if (val.length > 0 && hasContent) {
      target.classList.remove('hidden');
    } else {
      target.classList.add('hidden');
    }
  }
}
document.addEventListener('htmx:after:swap', handleOmnisearchSwap);
document.addEventListener('htmx:afterSwap', handleOmnisearchSwap);
document.addEventListener('htmx:after:settle', handleOmnisearchSwap);
document.addEventListener('htmx:afterSettle', handleOmnisearchSwap);
document.addEventListener('htmx:after:request', handleOmnisearchSwap);
document.addEventListener('htmx:afterRequest', handleOmnisearchSwap);

// DOM 変更の自動検知（HTMX のバージョン差異に依存しない安全設計）
document.addEventListener('DOMContentLoaded', () => {
  const dropdown = document.getElementById('omnisearch-dropdown');
  if (dropdown) {
    const observer = new MutationObserver(() => {
      const input = document.getElementById('omnisearch-input');
      const val = input ? input.value.trim() : '';
      if (val.length > 0 && dropdown.innerHTML.trim().length > 0) {
        dropdown.classList.remove('hidden');
      } else if (dropdown.innerHTML.trim().length === 0) {
        dropdown.classList.add('hidden');
      }
    });
    observer.observe(dropdown, { childList: true, subtree: true });
  }
});


// 3. 詳細ペインの開閉制御
// アイテムカードのクリック: 詳細ペインを開き、カードを選択状態にする（詳細の取得はカードの hx-get が行う）
function onItemCardClick(card) {
  openDetailPane(cardItemId(card));
  document.querySelectorAll('.item-card.selected').forEach(c => c.classList.remove('selected'));
  card.classList.add('selected');
}

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

// 4-2. サイドバー見出しの「+」からインライン追加フォームを開閉
function toggleSidebarAddForm(formId, show) {
  const form = document.getElementById(formId);
  if (!form) return;
  const open = show ?? form.classList.contains('hidden');
  form.classList.toggle('hidden', !open);
  const toggle = document.querySelector(`[aria-controls="${formId}"]`);
  if (toggle) toggle.setAttribute('aria-expanded', String(open));
  if (open) {
    form.querySelector('input')?.focus();
  } else {
    form.reset();
  }
}

// 4-3. 中央ペインに表示中のビューに合わせて、サイドバーの現在地ハイライトを付け替える
function syncActiveNav() {
  const viewEl = document.querySelector('#main-content-pane [data-view]');
  const sidebar = document.getElementById('sidebar-pane');
  if (!viewEl || !sidebar) return;

  const { view, projectId, contextId } = viewEl.dataset;
  const keys = new Set([view === 'project' && projectId ? `project-${projectId}` : view]);
  if (contextId) keys.add(`context-${contextId}`);

  sidebar.querySelectorAll('[data-nav]').forEach(el => {
    const active = keys.has(el.dataset.nav);
    el.classList.toggle('active', active);
    if (active) el.setAttribute('aria-current', 'page');
    else el.removeAttribute('aria-current');
  });
}
document.addEventListener('htmx:afterSwap', syncActiveNav);
document.addEventListener('htmx:after:swap', syncActiveNav);
document.addEventListener('DOMContentLoaded', syncActiveNav);

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

// 6. トースト / Snackbar 表示（M3 Snackbar: 任意で「元に戻す」等のアクションボタン付き）
function showToast(message, isError) {
  if (!message) return;
  message = decodeToastMessage(message);
  const duration = (message && message.length > 25) || isError ? 5000 : 2500;
  showSnackbar(message, { isError, duration });
}

function showSnackbar(message, { actionLabel, onAction, duration = 5000, isError = false } = {}) {
  let toast = document.getElementById('app-toast');
  if (!toast) {
    toast = document.createElement('div');
    toast.id = 'app-toast';
    toast.setAttribute('role', 'status');
    toast.setAttribute('aria-live', 'polite');
    document.body.appendChild(toast);
  }
  toast.className = 'toast-notice' + (isError ? ' toast-error' : '');
  toast.textContent = '';

  const text = document.createElement('span');
  text.className = 'toast-text';
  text.textContent = message;
  toast.appendChild(text);

  if (actionLabel && onAction) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'toast-action';
    btn.textContent = actionLabel;
    btn.onclick = () => {
      hideSnackbar();
      onAction();
    };
    toast.appendChild(btn);
  }

  toast.style.opacity = '1';
  toast.style.pointerEvents = 'auto';
  clearTimeout(toast._timeout);
  toast._timeout = setTimeout(hideSnackbar, duration);
}

function hideSnackbar() {
  const toast = document.getElementById('app-toast');
  if (!toast) return;
  clearTimeout(toast._timeout);
  toast.style.opacity = '0';
  toast.style.pointerEvents = 'none';
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

// 6-2. ステータス変更・削除の「元に戻す」
const STATUS_LABELS = {
  inbox: 'Inbox', next: 'Next Actions', waiting: 'Waiting For',
  someday: 'Someday', reference: 'Reference', done: 'Done',
};

// サーバーの変更後に関連ペイン（リスト・サイドバー・詳細・レビュー）を再描画させる
function refreshAfterUndo() {
  htmx.trigger(document.body, 'itemDetailRefresh');
  // 週次レビュー画面のカードは自動再描画されないため、表示中のステップを再取得
  const activeStep = document.querySelector('.review-container .step-tab.active');
  if (activeStep) {
    htmx.ajax('GET', activeStep.getAttribute('hx-get'), { target: '#main-content-pane' });
  }
}

async function sendItemRequest(method, url, values) {
  const opts = { method, headers: { 'HX-Request': 'true' } };
  if (values) {
    opts.headers['Content-Type'] = 'application/x-www-form-urlencoded';
    opts.body = new URLSearchParams(values).toString();
  }
  const res = await fetch(url, opts);
  if (!res.ok) throw new Error((await res.text()) || `リクエストに失敗しました (${res.status})`);
  dispatchHxTriggerHeader(res.headers.get('HX-Trigger'));
}

// fetch で送った場合も、htmx と同様に HX-Trigger レスポンスヘッダのイベントを発火する
function dispatchHxTriggerHeader(header) {
  if (!header) return;
  if (header.startsWith('{')) {
    for (const [name, value] of Object.entries(JSON.parse(header))) {
      htmx.trigger(document.body, name, typeof value === 'object' ? value : { value });
    }
  } else {
    header.split(',').forEach(name => htmx.trigger(document.body, name.trim()));
  }
}

document.addEventListener('itemStatusChanged', (e) => {
  const d = e.detail || {};
  if (!d.id) return;
  const title = d.title ? `「${d.title}」` : '';
  if (d.recurring) {
    showSnackbar(`${title}を完了し、次回分を作成しました`);
    return;
  }
  const verb = d.status === 'done' ? 'を完了しました' : `を ${STATUS_LABELS[d.status] || d.status} へ移動しました`;
  showSnackbar(title + verb, {
    actionLabel: '元に戻す',
    onAction: async () => {
      try {
        await sendItemRequest('PATCH', `/items/${d.id}/status`, { status: d.prev });
        refreshAfterUndo();
      } catch (err) {
        showToast(err.message, true);
      }
    },
  });
});

// 削除は即時に送らず、Snackbar の表示中は保留して「元に戻す」で取り消せるようにする
const pendingDeletes = new Map(); // itemId -> timeoutId
const DELETE_UNDO_MS = 5000;

function itemElementsFor(itemId) {
  return document.querySelectorAll(
    `[hx-get="/items/${itemId}/detail"].item-card, #review-card-${itemId}, #stale-card-${itemId}`
  );
}

function hidePendingDeletes() {
  for (const id of pendingDeletes.keys()) {
    itemElementsFor(id).forEach(el => el.classList.add('pending-delete'));
  }
}

function commitDelete(itemId, { keepalive = false } = {}) {
  clearTimeout(pendingDeletes.get(itemId));
  pendingDeletes.delete(itemId);
  const req = fetch(`/items/${itemId}`, { method: 'DELETE', headers: { 'HX-Request': 'true' }, keepalive });
  if (keepalive) return;
  req.then(res => {
    if (!res.ok) throw new Error('削除に失敗しました');
    htmx.trigger(document.body, 'itemChanged');
  }).catch(err => {
    showToast(err.message, true);
    itemElementsFor(itemId).forEach(el => el.classList.remove('pending-delete'));
  });
}

function deleteItemWithUndo(itemId, title) {
  itemId = String(itemId);
  const detailPane = document.getElementById('item-detail-pane');
  if (detailPane && detailPane.dataset.itemId === itemId) closeDetailPane();

  pendingDeletes.set(itemId, setTimeout(() => commitDelete(itemId), DELETE_UNDO_MS));
  hidePendingDeletes();
  handleReviewCardSwapped();

  showSnackbar(`${title ? `「${title}」` : 'アイテム'}を削除しました`, {
    duration: DELETE_UNDO_MS,
    actionLabel: '元に戻す',
    onAction: () => {
      clearTimeout(pendingDeletes.get(itemId));
      pendingDeletes.delete(itemId);
      itemElementsFor(itemId).forEach(el => el.classList.remove('pending-delete'));
      handleReviewCardSwapped();
    },
  });
}

// 一覧が再描画されても、削除保留中のアイテムは非表示のままにする
document.addEventListener('htmx:afterSwap', hidePendingDeletes);
document.addEventListener('htmx:after:swap', hidePendingDeletes);

// ページを離れる場合は保留中の削除を確定させる
window.addEventListener('pagehide', () => {
  for (const id of [...pendingDeletes.keys()]) commitDelete(id, { keepalive: true });
});

// HTMX リクエスト失敗時のエラー通知 (HTMX 4: htmx:response:error, HTMX 1/2: htmx:responseError)
function handleHtmxError(e) {
  const err = e.detail?.error;
  // 意図的なリクエスト中断（AbortError、Fetch is aborted、操作キャンセル等）はエラー表示しない
  if (err) {
    if (err.name === 'AbortError') return;
    const errStr = String(err.message || err).toLowerCase();
    if (errStr.includes('abort') || errStr.includes('cancel')) return;
  }

  const ctx = e.detail?.ctx;
  const xhr = e.detail?.xhr;
  const status = ctx?.response?.status || xhr?.status || 0;
  // ネットワーク未接続やブラウザキャンセルによる status 0 のエラーイベントも除外
  if (status === 0 && (!ctx?.text && !xhr?.responseText)) {
    return;
  }

  let msg = ctx?.text || xhr?.responseText || '';
  if (!msg && err) {
    msg = err.message || String(err);
  }
  if (!msg) {
    msg = status ? `リクエストに失敗しました (${status})` : 'エラーが発生しました';
  }
  showToast(msg.trim(), true);
}

document.addEventListener('htmx:response:error', handleHtmxError);
document.addEventListener('htmx:responseError', handleHtmxError);
document.addEventListener('htmx:error', handleHtmxError);

// 7. 週次レビュー Step 1 / Step 3 のカード削除時カウント・表示自動更新
function handleReviewCardSwapped(e) {
  const step1List = document.getElementById('step1-items-list');
  const step2List = document.getElementById('step2-projects-list');
  const step3List = document.getElementById('step3-items-list');
  if (!step1List && !step2List && !step3List) return;

  // スワップ対象の要素（e.detail.target または e.target）を取得
  const target = e?.detail?.target || e?.target;
  // レビュー画面に関係のないスワップ（サイドバー、オムニサーチ、モーダル等）ならスキップ
  if (target && (target.id === 'sidebar-pane' || target.id === 'omnisearch-dropdown' || target.id === 'modal-container' || target.id === 'item-detail-pane')) {
    return;
  }

  if (step1List) {
    const remaining = step1List.querySelectorAll('.review-card:not(.pending-delete)').length;
    const countEl = document.getElementById('step1-remaining-count');
    if (countEl) countEl.textContent = remaining;

    const badgeEl = document.getElementById('step1-tab-badge');
    if (badgeEl) {
      badgeEl.textContent = remaining;
      if (remaining === 0) {
        badgeEl.classList.remove('badge-alert');
      }
    }

    // 削除の「元に戻す」で件数が戻る場合もあるため、両方向に切り替える
    const headerEl = document.getElementById('step1-remaining-header');
    if (headerEl) headerEl.style.display = remaining === 0 ? 'none' : '';
    const zeroEl = document.getElementById('step1-zero-state');
    if (zeroEl) zeroEl.style.display = remaining === 0 ? 'block' : 'none';
  }

  if (step3List) {
    const remaining = step3List.querySelectorAll('.review-card:not(.pending-delete)').length;
    const countEl = document.getElementById('step3-remaining-count');
    if (countEl) countEl.textContent = remaining;

    // 削除の「元に戻す」で件数が戻る場合もあるため、両方向に切り替える
    const headerEl = document.getElementById('step3-remaining-header');
    if (headerEl) headerEl.style.display = remaining === 0 ? 'none' : '';
    const zeroEl = document.getElementById('step3-zero-state');
    if (zeroEl) zeroEl.style.display = remaining === 0 ? 'block' : 'none';
  }

  if (step2List) {
    const remaining = step2List.querySelectorAll('.review-card:not(.pending-delete)').length;
    if (remaining === 0 && !document.getElementById('step2-zero-state')) {
      const zeroDiv = document.createElement('div');
      zeroDiv.id = 'step2-zero-state';
      zeroDiv.className = 'review-card';
      zeroDiv.style.textAlign = 'center';
      zeroDiv.style.padding = '40px 20px';
      zeroDiv.innerHTML = '<h3 style="font-size: 1.1rem; font-weight: 600;">アクティブなプロジェクトはありません</h3><p style="font-size: 0.85rem; color: var(--md-sys-color-outline); margin-top: 4px;">新しいプロジェクトを作成するか、Step 3 へ進みましょう。</p>';
      step2List.appendChild(zeroDiv);
    }
  }
}
document.addEventListener('htmx:afterSwap', handleReviewCardSwapped);
document.addEventListener('htmx:after:swap', handleReviewCardSwapped);

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

// 9. 詳細メモ・手順の開閉とプレビュー同期
// 初期表示（メモがあると open 属性付きで描画される）でも toggle は発火するため、
// ユーザーが summary をクリックして開いたときだけ textarea へフォーカスする
function onDetailBodyToggle(detailsEl) {
  if (!detailsEl) return;
  const userToggled = detailsEl.dataset.userToggled === '1';
  delete detailsEl.dataset.userToggled;
  if (detailsEl.open && userToggled) {
    document.getElementById('detail-body')?.focus();
  }
}

function syncBodyPreview(value) {
  const snippetEl = document.getElementById('body-preview-snippet');
  if (!snippetEl) return;
  const trimmed = value.trim();
  if (trimmed.length > 0) {
    snippetEl.className = 'body-preview-snippet';
    snippetEl.title = trimmed;
    snippetEl.innerText = trimmed;
  } else {
    snippetEl.className = 'body-empty-hint';
    snippetEl.title = '';
    snippetEl.innerText = '+ メモを追加';
  }
}

// 9-2. 詳細ペインの日付入力（空なら「未設定」表示、クリアボタンで消去して自動保存）
function syncDateFieldEmpty(input) {
  const field = input.closest('.date-field');
  if (field) field.toggleAttribute('data-empty', !input.value);
}

// 日付のクイックチップ（今日 / 明日 / 来週月曜）。値を入れて change を発火し、既存の自動保存に乗せる
function localDateString(d) {
  // toISOString は UTC になるため使わない
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

function quickDate(kind, base = new Date()) {
  const d = new Date(base.getFullYear(), base.getMonth(), base.getDate());
  if (kind === 'tomorrow') d.setDate(d.getDate() + 1);
  if (kind === 'next-monday') d.setDate(d.getDate() + (((8 - d.getDay()) % 7) || 7));
  return localDateString(d);
}

function setDateField(inputId, kind) {
  const input = document.getElementById(inputId);
  if (!input) return;
  const value = quickDate(kind);
  if (input.value === value) return;
  input.value = value;
  syncDateFieldEmpty(input);
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

function clearDateField(inputId) {
  const input = document.getElementById(inputId);
  if (!input || !input.value) return;
  input.value = '';
  syncDateFieldEmpty(input);
  input.dispatchEvent(new Event('change', { bubbles: true }));
}

// 9-3. 詳細ペインの再描画（ステータス・タグ変更など）で、開閉状態とフォーカス・カーソル位置を保つ
const DETAIL_STATE_TTL_MS = 5000;
let detailStateSnapshot = null;

function swapTargetElement(e) {
  const t = e.detail?.ctx?.target ?? e.detail?.target;
  if (!t) return null;
  return typeof t === 'string' ? document.querySelector(t) : t;
}

function captureDetailState(e) {
  const target = swapTargetElement(e);
  if (!target || target.id !== 'item-detail-pane') return;
  const content = document.getElementById('item-detail-content');
  if (!content) return;

  const active = document.activeElement;
  const focus = active && content.contains(active) && active.id ? {
    id: active.id,
    start: typeof active.selectionStart === 'number' ? active.selectionStart : null,
    end: typeof active.selectionEnd === 'number' ? active.selectionEnd : null,
  } : null;

  detailStateSnapshot = {
    itemId: content.dataset.itemId,
    at: Date.now(),
    open: Object.fromEntries([...content.querySelectorAll('details[id]')].map(d => [d.id, d.open])),
    focus,
  };
}

function restoreDetailState() {
  const snap = detailStateSnapshot;
  if (!snap) return;
  const content = document.getElementById('item-detail-content');
  if (!content) return;
  detailStateSnapshot = null;
  if (content.dataset.itemId !== snap.itemId || Date.now() - snap.at > DETAIL_STATE_TTL_MS) return;

  for (const [id, open] of Object.entries(snap.open)) {
    const d = document.getElementById(id);
    if (d && d.open !== open) d.open = open;
  }
  if (snap.focus) {
    const el = document.getElementById(snap.focus.id);
    if (el) {
      el.focus({ preventScroll: true });
      if (snap.focus.start !== null && typeof el.setSelectionRange === 'function') {
        try { el.setSelectionRange(snap.focus.start, snap.focus.end); } catch (_) { /* type=date 等は非対応 */ }
      }
    }
  }
}

document.addEventListener('htmx:before:swap', captureDetailState);
document.addEventListener('htmx:beforeSwap', captureDetailState);
document.addEventListener('htmx:after:swap', restoreDetailState);
document.addEventListener('htmx:afterSwap', restoreDetailState);

// 9-4. 自動保存の状態表示（保存中… / 保存済み HH:MM / 保存に失敗しました）
function setDetailSaveStatus(state) {
  const el = document.getElementById('detail-save-status');
  if (!el) return;
  el.dataset.state = state;
  if (state === 'saving') {
    el.textContent = '保存中…';
  } else if (state === 'saved') {
    const now = new Date();
    el.textContent = `保存済み ${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`;
  } else {
    el.textContent = '保存に失敗しました';
  }
}

function isDetailFormRequest(e) {
  const src = e.detail?.ctx?.sourceElement ?? e.detail?.elt ?? e.target;
  return !!src?.closest?.('form.detail-form');
}

function onDetailSaveStart(e) {
  if (isDetailFormRequest(e)) setDetailSaveStatus('saving');
}

function onDetailSaveEnd(e) {
  if (!isDetailFormRequest(e)) return;
  const status = e.detail?.ctx?.response?.status ?? e.detail?.xhr?.status ?? 0;
  setDetailSaveStatus(status >= 200 && status < 400 ? 'saved' : 'error');
}

function onDetailSaveError(e) {
  if (isDetailFormRequest(e)) setDetailSaveStatus('error');
}

document.addEventListener('htmx:before:request', onDetailSaveStart);
document.addEventListener('htmx:beforeRequest', onDetailSaveStart);
document.addEventListener('htmx:after:request', onDetailSaveEnd);
document.addEventListener('htmx:afterRequest', onDetailSaveEnd);
document.addEventListener('htmx:error', onDetailSaveError);

// 9-5. テーマ切り替え（システム → ライト → ダーク）。初期反映は layout.html の <head> 内スクリプトが行う
const THEME_ORDER = ['system', 'light', 'dark'];
const THEME_LABELS = { system: 'システム設定に合わせる', light: 'ライト', dark: 'ダーク' };
const darkSchemeQuery = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;

function currentThemePref() {
  const pref = document.documentElement.dataset.themePref;
  return THEME_ORDER.includes(pref) ? pref : 'system';
}

function applyTheme(pref) {
  const root = document.documentElement;
  root.dataset.themePref = pref;
  if (pref === 'system') delete root.dataset.theme;
  else root.dataset.theme = pref;

  const isDark = pref === 'dark' || (pref === 'system' && !!darkSchemeQuery?.matches);
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', isDark ? '#111418' : '#00639b');

  const btn = document.getElementById('theme-toggle-btn');
  if (btn) btn.title = `テーマ: ${THEME_LABELS[pref]}（クリックで切り替え）`;
}

function cycleTheme() {
  const next = THEME_ORDER[(THEME_ORDER.indexOf(currentThemePref()) + 1) % THEME_ORDER.length];
  try {
    if (next === 'system') localStorage.removeItem('gtd_theme');
    else localStorage.setItem('gtd_theme', next);
  } catch (_) { /* 保存できなくても、このページ内では切り替える */ }
  applyTheme(next);
  showToast(`テーマ: ${THEME_LABELS[next]}`);
}

document.addEventListener('DOMContentLoaded', () => applyTheme(currentThemePref()));
darkSchemeQuery?.addEventListener?.('change', () => applyTheme(currentThemePref()));

// 10. モーダルダイアログ制御 (ランチャー管理など)
function openModal() {
  const container = document.getElementById('modal-container');
  if (container) {
    container.classList.remove('hidden');
    // 開いた後に名前入力欄へオートフォーカス
    setTimeout(() => {
      const nameInput = document.getElementById('launcher-name-input');
      if (nameInput) {
        nameInput.focus();
      }
    }, 50);
  }
}

function closeModal() {
  const container = document.getElementById('modal-container');
  if (container) {
    container.classList.add('hidden');
    container.innerHTML = '';
  }
}

function openLauncherModal() {
  const container = document.getElementById('modal-container');
  if (container) {
    openModal();
    if (window.htmx) {
      htmx.ajax('GET', '/partials/launchers', { target: '#modal-container', swap: 'innerHTML' });
    }
  }
}

// オムニサーチでランチャーを選択・起動した際のハンドラ
function onLauncherSelected() {
  closeOmnisearchDropdown();
  const searchInput = document.getElementById('omnisearch-input');
  if (searchInput) {
    searchInput.value = '';
    searchInput.blur();
  }
  const clearBtn = document.getElementById('omnisearch-clear-btn');
  if (clearBtn) {
    clearBtn.classList.add('hidden');
  }
}

// 11. ヘッダー時計 & タイマー & アラーム (Material Design 3 準拠 & localStorage 永続化)

// 状態変数群
let timerState = 'idle'; // 'idle', 'running', 'paused'
let timerDuration = 25 * 60; // 秒数 (デフォルト 25分)
let timerRemaining = 25 * 60;
let timerTargetTimestamp = 0; // 終了予定UNIXミリ秒
let timerMode = 'pomodoro'; // 'pomodoro', 'short_break', 'long_break', 'custom'

let alarmEnabled = false;
let alarmTime = ''; // "HH:MM"
let alarmDate = ''; // "YYYY-MM-DD"
let alarmRinging = false;
let alarmRingingTime = '';
let alarmLabel = ''; // 用件（任意のプレーンテキスト）
let alarmRingingLabel = '';
let currentAlertType = ''; // 'timer' | 'alarm' | ''

let isTimeAlertActive = false;
let alertIntervalId = null;
let originalDocumentTitle = document.title || 'GTD - Getting Things Done';
let audioCtx = null;

const clockWeekdays = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

// 日付文字列 (YYYY-MM-DD) の取得
function getTodayDateString() {
  const now = new Date();
  const y = now.getFullYear();
  const m = String(now.getMonth() + 1).padStart(2, '0');
  const d = String(now.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

// 秒数を "MM:SS" または "HH:MM:SS" にフォーマット
function formatSecondsToMMSS(totalSeconds) {
  const secs = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  const s = secs % 60;
  if (h > 0) {
    return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  }
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

// localStorage から状態を復元
function loadTimeStateFromStorage() {
  try {
    // 1. タイマーの読み込み
    const savedDuration = localStorage.getItem('gtd_timer_duration');
    if (savedDuration) {
      timerDuration = parseInt(savedDuration, 10) || (25 * 60);
    }

    const savedMode = localStorage.getItem('gtd_timer_mode');
    if (savedMode) {
      timerMode = savedMode;
    }

    const savedState = localStorage.getItem('gtd_timer_state');
    const savedTarget = localStorage.getItem('gtd_timer_target_timestamp');
    const savedRemaining = localStorage.getItem('gtd_timer_remaining');

    if (savedState === 'running' && savedTarget) {
      const target = parseInt(savedTarget, 10);
      const now = Date.now();
      if (target > now) {
        timerState = 'running';
        timerTargetTimestamp = target;
        timerRemaining = Math.max(0, Math.round((target - now) / 1000));
      } else {
        // バックグラウンド中に時間経過
        timerState = 'idle';
        timerRemaining = 0;
        localStorage.setItem('gtd_timer_state', 'idle');
        localStorage.setItem('gtd_timer_remaining', '0');
        localStorage.removeItem('gtd_timer_target_timestamp');
        setTimeout(() => {
          triggerTimeAlert('timer', 'タイマー終了', '設定したタイマー時間が終了しました');
        }, 500);
      }
    } else if (savedState === 'paused' && savedRemaining) {
      timerState = 'paused';
      timerRemaining = parseInt(savedRemaining, 10) || timerDuration;
    } else {
      timerState = 'idle';
      timerRemaining = timerDuration;
    }

    // 2. アラームの読み込み
    const savedAlarmEnabled = localStorage.getItem('gtd_alarm_enabled');
    const savedAlarmTime = localStorage.getItem('gtd_alarm_time');
    const savedAlarmDate = localStorage.getItem('gtd_alarm_date');
    const todayStr = getTodayDateString();

    if (savedAlarmEnabled === 'true' && savedAlarmTime && savedAlarmDate === todayStr) {
      const [ah, am] = savedAlarmTime.split(':').map(Number);
      const now = new Date();
      const targetAlarmTime = new Date(now.getFullYear(), now.getMonth(), now.getDate(), ah, am, 0);

      if (targetAlarmTime.getTime() > now.getTime()) {
        alarmEnabled = true;
        alarmTime = savedAlarmTime;
        alarmDate = savedAlarmDate;
        alarmLabel = localStorage.getItem('gtd_alarm_label') || '';
        const labelInput = document.getElementById('alarm-label-input');
        if (labelInput) labelInput.value = alarmLabel;
      } else {
        // 当日だがすでに過ぎた時刻
        alarmEnabled = false;
        alarmTime = '';
        alarmDate = '';
        localStorage.removeItem('gtd_alarm_enabled');
        localStorage.removeItem('gtd_alarm_time');
        localStorage.removeItem('gtd_alarm_date');
        localStorage.removeItem('gtd_alarm_label');
      }
    } else {
      alarmEnabled = false;
      alarmTime = '';
      alarmDate = '';
      localStorage.removeItem('gtd_alarm_enabled');
      localStorage.removeItem('gtd_alarm_time');
      localStorage.removeItem('gtd_alarm_date');
      localStorage.removeItem('gtd_alarm_label');
    }
  } catch (err) {
    console.error('Failed to load time state from localStorage', err);
  }
}

// モーダルの開閉とタブ切り替え
function openClockModal() {
  const modal = document.getElementById('clock-modal');
  if (!modal) return;
  modal.classList.remove('hidden');

  // もしアラーム鳴動中なら自動的にアラームタブを開く
  if (alarmRinging || (isTimeAlertActive && currentAlertType === 'alarm')) {
    switchClockTab('alarm');
  } else if (timerState === 'running' || (isTimeAlertActive && currentAlertType === 'timer')) {
    switchClockTab('timer');
  }

  updateTimerModalUI();
  updateAlarmModalUI();
}

function closeClockModal() {
  const modal = document.getElementById('clock-modal');
  if (modal) {
    modal.classList.add('hidden');
  }
}

function switchClockTab(tab) {
  const timerTabBtn = document.getElementById('tab-btn-timer');
  const alarmTabBtn = document.getElementById('tab-btn-alarm');
  const timerPanel = document.getElementById('clock-panel-timer');
  const alarmPanel = document.getElementById('clock-panel-alarm');

  if (tab === 'timer') {
    timerTabBtn?.classList.add('active');
    alarmTabBtn?.classList.remove('active');
    timerPanel?.classList.remove('hidden');
    alarmPanel?.classList.add('hidden');
  } else {
    alarmTabBtn?.classList.add('active');
    timerTabBtn?.classList.remove('active');
    alarmPanel?.classList.remove('hidden');
    timerPanel?.classList.add('hidden');
    ensureAlarmInputDefault();
  }
}

// --------------------------------------------------------------------------
// タイマー & ポモドーロ制御
// --------------------------------------------------------------------------

function selectTimerPreset(minutes, mode) {
  timerDuration = minutes * 60;
  timerRemaining = timerDuration;
  timerState = 'idle';
  timerMode = mode;

  localStorage.setItem('gtd_timer_duration', String(timerDuration));
  localStorage.setItem('gtd_timer_remaining', String(timerRemaining));
  localStorage.setItem('gtd_timer_mode', timerMode);
  localStorage.setItem('gtd_timer_state', 'idle');
  localStorage.removeItem('gtd_timer_target_timestamp');

  updateTimerModalUI();
  updateClockWidgetBadges();
}

function adjustTimer(deltaMinutes) {
  let currentMinutes = Math.floor(timerRemaining / 60);
  let newMinutes = currentMinutes + deltaMinutes;
  if (newMinutes < 5) newMinutes = 5;
  if (newMinutes > 180) newMinutes = 180;

  timerRemaining = newMinutes * 60;

  if (timerState === 'idle') {
    timerDuration = timerRemaining;
    timerMode = 'custom';
    localStorage.setItem('gtd_timer_duration', String(timerDuration));
    localStorage.setItem('gtd_timer_mode', timerMode);
  } else if (timerState === 'running') {
    timerTargetTimestamp = Date.now() + timerRemaining * 1000;
    localStorage.setItem('gtd_timer_target_timestamp', String(timerTargetTimestamp));
  }

  localStorage.setItem('gtd_timer_remaining', String(timerRemaining));
  updateTimerModalUI();
  updateClockWidgetBadges();
}

function toggleTimer() {
  if (timerState === 'running') {
    // 一時停止
    timerState = 'paused';
    timerRemaining = Math.max(0, Math.round((timerTargetTimestamp - Date.now()) / 1000));
    localStorage.setItem('gtd_timer_state', 'paused');
    localStorage.setItem('gtd_timer_remaining', String(timerRemaining));
    localStorage.removeItem('gtd_timer_target_timestamp');
  } else {
    // 開始または再開
    if (timerRemaining <= 0) {
      timerRemaining = timerDuration;
    }
    timerState = 'running';
    timerTargetTimestamp = Date.now() + timerRemaining * 1000;
    localStorage.setItem('gtd_timer_state', 'running');
    localStorage.setItem('gtd_timer_target_timestamp', String(timerTargetTimestamp));
    localStorage.removeItem('gtd_timer_remaining');
  }

  updateTimerModalUI();
  updateClockWidgetBadges();
}

function resetTimer() {
  timerState = 'idle';
  timerRemaining = timerDuration;

  localStorage.setItem('gtd_timer_state', 'idle');
  localStorage.setItem('gtd_timer_remaining', String(timerRemaining));
  localStorage.removeItem('gtd_timer_target_timestamp');

  // タイマー鳴動中ならアラート停止
  if (currentAlertType === 'timer') {
    dismissTimeAlert();
  }

  updateTimerModalUI();
  updateClockWidgetBadges();
}

function updateTimerModalUI() {
  const countdownEl = document.getElementById('timer-countdown');
  const toggleBtn = document.getElementById('timer-toggle-btn');
  const toggleText = document.getElementById('timer-toggle-text');
  const toggleIcon = document.getElementById('timer-toggle-icon');
  const progressBar = document.getElementById('timer-progress-bar');
  const modeLabel = document.getElementById('timer-mode-label');

  if (countdownEl) {
    countdownEl.textContent = formatSecondsToMMSS(timerRemaining);
  }

  if (progressBar) {
    const pct = timerDuration > 0 ? Math.min(100, Math.max(0, (timerRemaining / timerDuration) * 100)) : 100;
    progressBar.style.width = `${pct}%`;
  }

  if (modeLabel) {
    const mins = Math.round(timerDuration / 60);
    if (timerMode === 'pomodoro') {
      modeLabel.textContent = `ポモドーロ (${mins}分)`;
    } else if (timerMode === 'short_break') {
      modeLabel.textContent = `小休憩 (${mins}分)`;
    } else if (timerMode === 'long_break') {
      modeLabel.textContent = `長休憩 (${mins}分)`;
    } else {
      modeLabel.textContent = `タイマー (${mins}分)`;
    }
  }

  // プリセットチップのアクティブ状態
  document.querySelectorAll('.timer-chips-row .m3-chip').forEach(c => c.classList.remove('active'));
  if (timerMode === 'pomodoro' && timerDuration === 25 * 60) {
    document.getElementById('chip-pomo-25')?.classList.add('active');
  } else if (timerMode === 'short_break' && timerDuration === 5 * 60) {
    document.getElementById('chip-pomo-short')?.classList.add('active');
  } else if (timerMode === 'long_break' && timerDuration === 15 * 60) {
    document.getElementById('chip-pomo-long')?.classList.add('active');
  } else if (timerDuration === 30 * 60) {
    document.getElementById('chip-preset-30')?.classList.add('active');
  } else if (timerDuration === 45 * 60) {
    document.getElementById('chip-preset-45')?.classList.add('active');
  } else if (timerDuration === 60 * 60) {
    document.getElementById('chip-preset-60')?.classList.add('active');
  }

  if (toggleText && toggleIcon) {
    if (timerState === 'running') {
      toggleText.textContent = '一時停止';
      toggleIcon.innerHTML = '<path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/>';
    } else {
      toggleText.textContent = timerState === 'paused' ? '再開' : '開始';
      toggleIcon.innerHTML = '<path d="M8 5v14l11-7z"/>';
    }
  }
}

// --------------------------------------------------------------------------
// アラーム制御 (当日限定 & デフォルト現在時刻+1時間)
// --------------------------------------------------------------------------

function ensureAlarmInputDefault() {
  const input = document.getElementById('alarm-time-input');
  if (!input) return;
  if (!input.value) {
    setAlarmOffset(60);
  } else {
    validateAlarmTime(input.value);
  }
}

function setAlarmOffset(offsetMinutes) {
  const now = new Date();
  const target = new Date(now.getTime() + offsetMinutes * 60 * 1000);

  // 当日限定：日をまたぐ場合は 23:59 に丸める
  if (target.getDate() !== now.getDate()) {
    target.setHours(23, 59, 0, 0);
  }

  const h = String(target.getHours()).padStart(2, '0');
  const m = String(target.getMinutes()).padStart(2, '0');
  const timeStr = `${h}:${m}`;

  const input = document.getElementById('alarm-time-input');
  if (input) {
    input.value = timeStr;
  }

  // クイック指定チップのアクティブ制御
  document.querySelectorAll('.alarm-presets-section .m3-chip').forEach(c => c.classList.remove('active'));
  if (offsetMinutes === 60) {
    document.getElementById('chip-alarm-default')?.classList.add('active');
  }

  validateAlarmTime(timeStr);
}

function validateAlarmTime(timeStr) {
  const msgEl = document.getElementById('alarm-validation-msg');
  const setBtn = document.getElementById('alarm-set-btn');

  if (!timeStr) {
    if (msgEl) {
      msgEl.textContent = '※当日の現在時刻より後の時刻を設定してください';
      msgEl.style.color = 'var(--md-sys-color-on-surface-variant)';
    }
    if (setBtn) setBtn.disabled = true;
    return false;
  }

  const [h, m] = timeStr.split(':').map(Number);
  const now = new Date();
  const targetDate = new Date(now.getFullYear(), now.getMonth(), now.getDate(), h, m, 0);

  if (targetDate.getTime() <= now.getTime()) {
    if (msgEl) {
      msgEl.textContent = '※当日の現在時刻より後の時刻を指定してください (過去の時刻は無効です)';
      msgEl.style.color = 'var(--md-sys-color-error)';
    }
    if (setBtn) setBtn.disabled = true;
    return false;
  }

  const diffMinutes = Math.round((targetDate.getTime() - now.getTime()) / 60000);
  const diffHours = Math.floor(diffMinutes / 60);
  const remainingMins = diffMinutes % 60;
  let diffStr = '';
  if (diffHours > 0) {
    diffStr = `${diffHours}時間${remainingMins > 0 ? remainingMins + '分' : ''}`;
  } else {
    diffStr = `${remainingMins}分`;
  }

  if (msgEl) {
    msgEl.textContent = `現在から約 ${diffStr} 後にアラームが鳴ります`;
    msgEl.style.color = 'var(--md-sys-color-primary)';
  }
  if (setBtn) setBtn.disabled = false;
  return true;
}

function onAlarmInputChange() {
  const input = document.getElementById('alarm-time-input');
  if (input) {
    validateAlarmTime(input.value);
  }
}

function saveAlarmFromInput() {
  const input = document.getElementById('alarm-time-input');
  if (!input || !validateAlarmTime(input.value)) return;

  alarmTime = input.value;
  alarmDate = getTodayDateString();
  alarmLabel = (document.getElementById('alarm-label-input')?.value || '').trim();
  alarmEnabled = true;
  alarmRinging = false;
  alarmRingingTime = '';
  alarmRingingLabel = '';

  localStorage.setItem('gtd_alarm_enabled', 'true');
  localStorage.setItem('gtd_alarm_time', alarmTime);
  localStorage.setItem('gtd_alarm_date', alarmDate);
  if (alarmLabel) {
    localStorage.setItem('gtd_alarm_label', alarmLabel);
  } else {
    localStorage.removeItem('gtd_alarm_label');
  }

  updateAlarmModalUI();
  updateClockWidgetBadges();
  showToast(`アラームを ${alarmTime} にセットしました${alarmLabel ? `（${alarmLabel}）` : ''}`);
}

function clearAlarm() {
  const wasRinging = alarmRinging;
  const wasActive = alarmEnabled;

  alarmEnabled = false;
  alarmTime = '';
  alarmDate = '';
  alarmLabel = '';
  alarmRinging = false;
  alarmRingingTime = '';
  alarmRingingLabel = '';

  localStorage.removeItem('gtd_alarm_enabled');
  localStorage.removeItem('gtd_alarm_time');
  localStorage.removeItem('gtd_alarm_date');
  localStorage.removeItem('gtd_alarm_label');

  // アラート作動中なら確実に停止
  dismissTimeAlert();

  // 入力欄も現在時刻の1時間後（デフォルト）にリセットし、用件も空にする
  setAlarmOffset(60);
  const labelInput = document.getElementById('alarm-label-input');
  if (labelInput) labelInput.value = '';

  updateAlarmModalUI();
  updateClockWidgetBadges();

  if (wasRinging) {
    showToast('アラームを停止しました');
  } else if (wasActive) {
    showToast('アラームを解除しました');
  }
}

function updateAlarmModalUI() {
  const input = document.getElementById('alarm-time-input');
  const statusBox = document.getElementById('alarm-status-box');
  const headline = document.getElementById('alarm-status-headline');
  const detail = document.getElementById('alarm-status-detail');
  const cancelBtn = document.getElementById('alarm-cancel-btn');
  const cancelBtnText = document.getElementById('alarm-cancel-btn-text');

  if (alarmRinging) {
    // 鳴動中・終了した状態
    statusBox?.classList.add('active', 'ringing');
    if (headline) headline.textContent = `アラーム到達: ${alarmRingingTime || alarmTime}${alarmRingingLabel ? ` — ${alarmRingingLabel}` : ''}`;
    if (detail) detail.textContent = '設定時刻になりました。「解除」ボタンで停止できます';
    if (cancelBtn) {
      cancelBtn.disabled = false;
      cancelBtn.title = 'アラームを停止・解除';
    }
    if (cancelBtnText) {
      cancelBtnText.textContent = 'アラーム停止 / 解除';
    }
  } else if (alarmEnabled && alarmTime) {
    // セット中
    if (input) input.value = alarmTime;
    const labelInput = document.getElementById('alarm-label-input');
    if (labelInput && document.activeElement !== labelInput) labelInput.value = alarmLabel;
    statusBox?.classList.remove('ringing');
    statusBox?.classList.add('active');
    if (headline) headline.textContent = `アラーム設定中: ${alarmTime}${alarmLabel ? ` — ${alarmLabel}` : ''}`;

    const [h, m] = alarmTime.split(':').map(Number);
    const now = new Date();
    const target = new Date(now.getFullYear(), now.getMonth(), now.getDate(), h, m, 0);
    const diffMin = Math.max(0, Math.round((target.getTime() - now.getTime()) / 60000));
    if (detail) detail.textContent = `本日 ${alarmTime} に通知します (残り 約${diffMin}分)`;
    if (cancelBtn) {
      cancelBtn.disabled = false;
      cancelBtn.title = 'アラームを解除';
    }
    if (cancelBtnText) {
      cancelBtnText.textContent = '解除';
    }
  } else {
    // 未設定
    statusBox?.classList.remove('active', 'ringing');
    if (headline) headline.textContent = 'アラームは未設定です';
    if (detail) detail.textContent = '時刻を指定して「アラームをセット」を押してください';
    if (cancelBtn) {
      cancelBtn.disabled = true;
      cancelBtn.title = '解除';
    }
    if (cancelBtnText) {
      cancelBtnText.textContent = '解除';
    }
    if (input) {
      // 過去の時刻が残っているか空の場合は、現在時刻+1時間後に自動リセット
      if (!input.value || !validateAlarmTime(input.value)) {
        setAlarmOffset(60);
      }
    }
  }
}

// --------------------------------------------------------------------------
// タイムアップ・アラーム到達通知 (視覚変化 & チャイム)
// --------------------------------------------------------------------------

function playSoftChime() {
  try {
    const AudioContext = window.AudioContext || window.webkitAudioContext;
    if (!AudioContext) return;
    if (!audioCtx) audioCtx = new AudioContext();
    if (audioCtx.state === 'suspended') {
      audioCtx.resume();
    }

    const now = audioCtx.currentTime;
    const osc = audioCtx.createOscillator();
    const gain = audioCtx.createGain();

    osc.type = 'sine';
    // 柔らかな2音チャイム (587Hz -> 880Hz)
    osc.frequency.setValueAtTime(587.33, now);
    osc.frequency.setValueAtTime(880, now + 0.15);

    gain.gain.setValueAtTime(0, now);
    gain.gain.linearRampToValueAtTime(0.18, now + 0.04);
    gain.gain.exponentialRampToValueAtTime(0.001, now + 0.6);

    osc.connect(gain);
    gain.connect(audioCtx.destination);

    osc.start(now);
    osc.stop(now + 0.6);
  } catch (err) {
    // オートプレイ制限環境では静かにフォールバック
  }
}

function triggerTimeAlert(type, title, message) {
  isTimeAlertActive = true;
  currentAlertType = type;

  // 1. ヘッダー時計ウィジェットのパルス発光
  const clockEl = document.getElementById('topbar-clock');
  clockEl?.classList.add('ringing');

  // 2. 画面上部へのフローティング警告バナー表示
  const banner = document.getElementById('time-alert-banner');
  const titleEl = document.getElementById('time-alert-title');
  const msgEl = document.getElementById('time-alert-message');
  if (banner) banner.classList.remove('hidden');
  if (titleEl) titleEl.textContent = title;
  if (msgEl) msgEl.textContent = message;

  // 3. ブラウザタブタイトルの点滅
  if (alertIntervalId) clearInterval(alertIntervalId);
  let isAlertTitle = true;
  alertIntervalId = setInterval(() => {
    document.title = isAlertTitle ? `【時間です】${title}` : originalDocumentTitle;
    isAlertTitle = !isAlertTitle;
  }, 1000);

  // 4. ピュアJSソフトチャイム鳴動
  playSoftChime();
}

function dismissTimeAlert() {
  isTimeAlertActive = false;
  currentAlertType = '';
  alarmRinging = false;
  alarmRingingTime = '';
  alarmRingingLabel = '';

  const clockEl = document.getElementById('topbar-clock');
  clockEl?.classList.remove('ringing');

  const banner = document.getElementById('time-alert-banner');
  banner?.classList.add('hidden');

  if (alertIntervalId) {
    clearInterval(alertIntervalId);
    alertIntervalId = null;
  }
  document.title = originalDocumentTitle;

  // アラームが終了した場合は、入力欄を現在時刻+1時間後にリセット
  if (!alarmEnabled) {
    setAlarmOffset(60);
    const labelInput = document.getElementById('alarm-label-input');
    if (labelInput) labelInput.value = '';
  }

  updateAlarmModalUI();
  updateTimerModalUI();
  updateClockWidgetBadges();
}

// --------------------------------------------------------------------------
// ヘッダー時計ウィジェットのバッジ更新
// --------------------------------------------------------------------------

function updateClockWidgetBadges() {
  const timerBadge = document.getElementById('clock-badge-timer');
  const timerBadgeText = document.getElementById('clock-badge-timer-text');
  const alarmBadge = document.getElementById('clock-badge-alarm');
  const alarmBadgeText = document.getElementById('clock-badge-alarm-text');

  // タイマーバッジ
  if (timerBadge && timerBadgeText) {
    if (timerState === 'running' || timerState === 'paused') {
      timerBadge.classList.remove('hidden');
      timerBadge.style.display = 'inline-flex';
      timerBadgeText.textContent = formatSecondsToMMSS(timerRemaining);
      if (timerState === 'paused') {
        timerBadge.title = `タイマー一時停止中 (${formatSecondsToMMSS(timerRemaining)})`;
        timerBadge.style.opacity = '0.75';
      } else {
        timerBadge.title = `タイマー動作中 (残り ${formatSecondsToMMSS(timerRemaining)})`;
        timerBadge.style.opacity = '1';
      }
    } else {
      timerBadge.classList.add('hidden');
      timerBadge.style.display = 'none';
    }
  }

  // アラームバッジ
  if (alarmBadge && alarmBadgeText) {
    if (alarmEnabled && alarmTime) {
      alarmBadge.classList.remove('hidden');
      alarmBadge.style.display = 'inline-flex';
      alarmBadgeText.textContent = alarmLabel ? `${alarmTime} ${alarmLabel}` : alarmTime;
      alarmBadge.title = `本日 ${alarmTime} にアラーム設定中${alarmLabel ? `: ${alarmLabel}` : ''}`;
    } else {
      alarmBadge.classList.add('hidden');
      alarmBadge.style.display = 'none';
      alarmBadgeText.textContent = '--:--';
    }
  }
}

// --------------------------------------------------------------------------
// 時計初期化 ＆ 毎秒ループ処理
// --------------------------------------------------------------------------

function initClock() {
  const dateEl = document.getElementById('clock-date');
  const timeEl = document.getElementById('clock-time');
  if (!dateEl || !timeEl) return;

  // localStorage から状態を復旧
  loadTimeStateFromStorage();

  function tick() {
    const now = new Date();
    const year = now.getFullYear();
    const month = String(now.getMonth() + 1).padStart(2, '0');
    const day = String(now.getDate()).padStart(2, '0');
    const weekday = clockWeekdays[now.getDay()];

    const hours = String(now.getHours()).padStart(2, '0');
    const minutes = String(now.getMinutes()).padStart(2, '0');

    dateEl.textContent = `${year}-${month}-${day} ${weekday}`;
    const clockBtn = document.getElementById('topbar-clock');
    if (clockBtn) clockBtn.title = `${year}-${month}-${day} ${weekday} — 時計・タイマー・アラーム設定`;
    
    const hoursEl = document.getElementById('clock-hours');
    const minutesEl = document.getElementById('clock-minutes');
    if (hoursEl && minutesEl) {
      hoursEl.textContent = hours;
      minutesEl.textContent = minutes;
    } else {
      timeEl.innerHTML = `<span id="clock-hours">${hours}</span><span class="clock-colon" id="clock-colon">:</span><span id="clock-minutes">${minutes}</span>`;
    }

    // 1. タイマーカウントダウン判定
    if (timerState === 'running') {
      const remaining = Math.max(0, Math.round((timerTargetTimestamp - Date.now()) / 1000));
      timerRemaining = remaining;
      if (timerRemaining <= 0) {
        timerState = 'idle';
        localStorage.setItem('gtd_timer_state', 'idle');
        localStorage.setItem('gtd_timer_remaining', '0');
        localStorage.removeItem('gtd_timer_target_timestamp');
        updateTimerModalUI();
        triggerTimeAlert('timer', 'タイマー終了', `${Math.round(timerDuration / 60)}分のタイマーが終了しました`);
      } else {
        updateTimerModalUI();
      }
    }

    // 2. アラーム到達判定 (目標時刻到達で発火)
    if (alarmEnabled && alarmTime) {
      const [ah, am] = alarmTime.split(':').map(Number);
      const targetAlarmTime = new Date(now.getFullYear(), now.getMonth(), now.getDate(), ah, am, 0, 0);

      if (now.getTime() >= targetAlarmTime.getTime()) {
        const ringTime = alarmTime;
        alarmEnabled = false;
        alarmRinging = true;
        alarmRingingTime = ringTime;
        alarmRingingLabel = alarmLabel;
        alarmLabel = '';

        localStorage.removeItem('gtd_alarm_enabled');
        localStorage.removeItem('gtd_alarm_time');
        localStorage.removeItem('gtd_alarm_date');
        localStorage.removeItem('gtd_alarm_label');

        updateAlarmModalUI();
        updateClockWidgetBadges();
        triggerTimeAlert('alarm', alarmRingingLabel || 'アラーム時刻到達', alarmRingingLabel ? `アラーム時刻 (${ringTime}) になりました` : `指定の時刻 (${ringTime}) になりました`);
      }
    }

    // 3. ウィジェット内のミニバッジ更新
    updateClockWidgetBadges();
  }

  tick();
  setInterval(tick, 1000);
}

// ページロード時に時計を初期化
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initClock);
} else {
  initClock();
}



