/* 开心十三张 · 牌桌前端。所有规则判定都在 Go 服务端完成，这里只负责展示与操作。 */
(() => {
  'use strict';

  const SUIT_SYMBOL = ['♦', '♣', '♥', '♠'];
  const SUIT_IS_RED = [true, false, true, false];
  const RANK_LABEL = { 11: 'J', 12: 'Q', 13: 'K', 14: 'A' };
  const PLACE_LABEL = ['', '第 1 名', '第 2 名', '第 3 名', '第 4 名'];
  const HUMAN = 0;
  const POLL_MS = 400;

  const $ = (id) => document.getElementById(id);
  const el = {
    roundNo: $('roundNo'), bet: $('bet'), turnTip: $('turnTip'),
    hand: $('hand'), status: $('status'), scores: $('scores'), log: $('log'),
    toBeat: $('toBeat'), playBtn: $('playBtn'), passBtn: $('passBtn'), hintBtn: $('hintBtn'),
    nextBtn: $('nextBtn'), resetBtn: $('resetBtn'), rulesBtn: $('rulesBtn'), closeRules: $('closeRules'),
    rulesModal: $('rulesModal'), resultModal: $('resultModal'), resultTitle: $('resultTitle'),
    resultBody: $('resultBody'), againBtn: $('againBtn'), toast: $('toast'),
    intro: $('intro'),
  };

  const state = {
    view: null,
    cursor: 0,
    roundSeen: null,
    selected: new Set(),
    seatShows: {},
    resultShownFor: null,
    introShown: false,
    busy: false,
  };

  const key = (c) => c.suit + '-' + c.rank;
  const rankLabel = (r) => RANK_LABEL[r] || String(r);

  function cardEl(c, small) {
    const d = document.createElement('div');
    d.className = 'card' + (small ? ' small' : '') + (SUIT_IS_RED[c.suit] ? ' red' : '');
    d.dataset.key = key(c);
    const s = SUIT_SYMBOL[c.suit], r = rankLabel(c.rank);
    d.innerHTML = `<span class="r">${r}</span><span class="s">${s}</span><span class="pip">${s}</span>`;
    return d;
  }

  function backEl() {
    const d = document.createElement('div');
    d.className = 'card back small';
    return d;
  }

  function cardRow(cards, small) {
    const wrap = document.createDocumentFragment();
    (cards || []).forEach((c) => wrap.appendChild(cardEl(c, small)));
    return wrap;
  }

  /* ---------- polling ---------- */

  async function poll() {
    if (state.busy) return;
    state.busy = true;
    try {
      let res = await fetch('/api/state?since=' + state.cursor);
      let data = await res.json();
      if (data.view && state.roundSeen !== null && data.view.roundNo !== state.roundSeen) {
        // 新的一局的事件序号从 1 重新开始，旧游标会漏掉整局，退回 0 重取。
        resetForNewRound();
        res = await fetch('/api/state?since=0');
        data = await res.json();
      }
      if (!data.ok) throw new Error(data.error || '服务器返回异常');
      applyEvents(data.events || []);
      if (data.view) {
        state.view = data.view;
        state.cursor = data.view.seq;
        state.roundSeen = data.view.roundNo;
      }
      render();
    } catch (err) {
      setStatus('与游戏服务的连接中断：' + err.message, true);
    } finally {
      state.busy = false;
    }
  }

  function resetForNewRound() {
    state.cursor = 0;
    state.seatShows = {};
    state.selected.clear();
    el.resultModal.hidden = true;
    state.resultShownFor = null;
  }

  function applyEvents(events) {
    events.forEach((ev) => {
      switch (ev.type) {
        case 'deal':
          state.seatShows = {};
          break;
        case 'play':
          state.seatShows[ev.seat] = { cards: ev.cards, kind: ev.kind };
          break;
        case 'pass':
          state.seatShows[ev.seat] = { pass: true };
          break;
        case 'trick':
          state.seatShows = {};
          break;
        default:
          break;
      }
      logEvent(ev);
    });
  }

  function logEvent(ev) {
    const line = document.createElement('div');
    if (ev.type === 'trick' || ev.type === 'deal') line.className = 'sep';
    if (ev.type === 'over') line.className = 'sep';
    line.textContent = ev.text || describe(ev);
    el.log.appendChild(line);
    while (el.log.childNodes.length > 220) el.log.removeChild(el.log.firstChild);
    el.log.scrollTop = el.log.scrollHeight;
  }

  function describe(ev) {
    const who = seatName(ev.seat);
    if (ev.type === 'play') return `${who} 出 ${ev.kind || '牌'}`;
    if (ev.type === 'pass') return `${who} 不出`;
    return who;
  }

  /* ---------- render ---------- */

  function seat(seatId) {
    return state.view ? state.view.seats[seatId] : null;
  }

  function seatName(seatId) {
    const s = seat(seatId);
    return s ? s.name : '座位 ' + seatId;
  }

  function render() {
    if (!state.view) return;
    const v = state.view;
    playIntro(v);
    el.roundNo.textContent = v.roundNo;
    renderBeat();
    [0, 1, 2, 3].forEach(renderSeat);
    renderHand();
    renderScores();
    renderControls();
    renderResult();
  }

  // 入场动画只属于第一次开局：整场游戏只在 roundNo === 1 的首帧放一次，之后按确定开
  // 下一局都不会再放。动画盖在牌桌上面，放完自己收掉，不拦点击。
  function playIntro(v) {
    if (state.introShown || v.roundNo !== 1) {
      return;
    }
    state.introShown = true;
    el.intro.hidden = false;
    el.intro.addEventListener('animationend', () => { el.intro.hidden = true; }, { once: true });
  }

  function renderBeat() {
    el.toBeat.innerHTML = '';
    const v = state.view;
    if (!v) return;
    const tag = document.createElement('div');
    tag.className = 'tag';
    if (v.phase === 'over') {
      tag.textContent = '本局结束';
      el.toBeat.appendChild(tag);
      return;
    }
    if (v.table) {
      tag.textContent = `要压过 ${seatName(v.table.seat)} 的${v.table.name}`;
      el.toBeat.appendChild(tag);
      el.toBeat.appendChild(cardRow(v.table.cards, false));
      return;
    }
    tag.textContent = v.turn === HUMAN ? '轮到你先出' : `${seatName(v.turn)} 即将先出`;
    el.toBeat.appendChild(tag);
  }

  function renderSeat(seatId) {
    const holder = document.querySelector(`.seat-${seatId}`);
    if (!holder || !seat(seatId)) return;
    const v = state.view, s = seat(seatId);
    holder.innerHTML = '';
    holder.classList.toggle('active', v.phase === 'playing' && v.turn === seatId);

    const plate = document.createElement('div');
    plate.className = 'plate';
    const avatar = document.createElement('div');
    avatar.className = 'avatar';
    const meta = s.place
      ? `<span class="medal">${PLACE_LABEL[s.place]}</span>`
      : `剩 ${s.count} 张`;
    avatar.innerHTML = `<span class="who">${s.name}${s.isBot ? '' : '（你）'}</span><span class="meta">${meta}</span>`;
    plate.appendChild(avatar);
    holder.appendChild(plate);

    if (s.isBot && !s.place) {
      const backs = document.createElement('div');
      backs.className = 'backs';
      for (let i = 0; i < Math.min(s.count, 13); i++) backs.appendChild(backEl());
      holder.appendChild(backs);
    }

    const show = state.seatShows[seatId];
    if (show) {
      const played = document.createElement('div');
      played.className = 'played big';
      if (show.pass) {
        const p = document.createElement('div');
        p.className = 'passed';
        p.textContent = '不出';
        played.appendChild(p);
      } else {
        played.appendChild(cardRow(show.cards, false));
      }
      holder.appendChild(played);
    }
  }

  function renderHand() {
    const s = seat(HUMAN);
    if (!s) return;
    const hand = s.hand || [];
    const alive = new Set(hand.map(key));
    [...state.selected].forEach((k) => { if (!alive.has(k)) state.selected.delete(k); });
    el.hand.innerHTML = '';
    hand.forEach((c) => {
      const d = cardEl(c, false);
      d.classList.toggle('sel', state.selected.has(key(c)));
      d.addEventListener('click', () => {
        if (state.view && state.view.turn === HUMAN && state.view.phase === 'playing') {
          state.selected.has(key(c)) ? state.selected.delete(key(c)) : state.selected.add(key(c));
          renderHand();
          renderControls();
        }
      });
      el.hand.appendChild(d);
    });
  }

  function renderScores() {
    if (!state.view) return;
    el.scores.innerHTML = '';
    const head = ['玩家', '积分', '本局'];
    head.forEach((h) => {
      const d = document.createElement('div');
      d.className = 'hd';
      d.textContent = h;
      el.scores.appendChild(d);
    });
    state.view.seats.forEach((s) => {
      const name = document.createElement('div');
      name.textContent = s.place ? `${s.name}（${PLACE_LABEL[s.place]}）` : s.name;
      // 积分列报的是手上的总分，起手 1000，1 分 = 1 倍下注。
      const total = document.createElement('div');
      total.textContent = points(s.points);
      const over = state.view.phase === 'over';
      const delta = document.createElement('div');
      delta.textContent = over ? signed(s.delta) : '—';
      delta.className = s.delta > 0 ? 'plus' : s.delta < 0 ? 'minus' : '';
      el.scores.append(name, total, delta);
    });
  }

  // 1 分 = 1 倍下注，所以第二、三名是本局 -0.5 分这种半分：整数不带小数点，半分保留一位。
  // 本局的输赢写成带符号的积分；起手 1000 分之后没有人是负数，所以总分不带正号。
  const points = (n) => Number(n).toFixed(1).replace(/\.0$/, '');
  const signed = (n) => (n > 0 ? '+' : '') + points(n);

  function renderControls() {
    const v = state.view;
    const mine = v && v.phase === 'playing' && v.turn === HUMAN;
    el.playBtn.disabled = !mine || state.selected.size === 0;
    el.passBtn.disabled = !mine || !v.canPass;
    el.hintBtn.disabled = !mine;
    el.bet.disabled = !(v && v.phase === 'over');
    if (v) {
      el.bet.value = String(v.bet);
      el.turnTip.textContent = v.phase === 'over'
        ? '本局结束'
        : (mine ? '轮到你出牌' : `等待 ${seatName(v.turn)}…`);
      el.turnTip.classList.toggle('mine', !!mine);
    }
  }

  function renderResult() {
    const v = state.view;
    if (!v || v.phase !== 'over' || !v.result) return;
    const stamp = v.roundNo + '-' + v.result.ranks.map((r) => r.seat).join('');
    if (state.resultShownFor === stamp) return;
    state.resultShownFor = stamp;
    const human = v.result.ranks.find((r) => r.seat === HUMAN);
    el.resultTitle.textContent = human.place === 1 ? '🎉 你是赢家！' : `本局结束 · 你获得第 ${human.place} 名`;
    el.resultBody.innerHTML = `<table class="result"><tr><th>名次</th><th>玩家</th><th>本局输赢</th><th>手上积分</th></tr>` +
      v.result.ranks.map((r) =>
        `<tr class="${r.place === 1 ? 'win' : ''}"><td>${PLACE_LABEL[r.place]}</td><td>${r.name}${r.seat === HUMAN ? '（你）' : ''}</td>` +
        `<td>${signed(r.delta)}</td><td>${points(r.points)}</td></tr>`).join('') +
      `</table><p class="muted">下注 ${fmt(v.bet)} 倍 · 第 ${v.result.roundNo} 局 · 第二名与第三名各输自己下注的 2/4，最后一名输掉其他三家的下注之和。每家起手 1000 分，1 分 = 1 倍，所以 1 倍时第一名 +4、第二三名各 -0.5、最后一名 -3。</p>`;
    el.resultModal.hidden = false;
  }

  const fmt = (n) => (n > 0 ? '+' : '') + Number(n).toFixed(1).replace(/\.0$/, '');

  function setStatus(text, warn) {
    el.status.textContent = text;
    el.status.classList.toggle('warn', !!warn);
  }

  let toastTimer = null;
  function toast(text, ok) {
    el.toast.textContent = text;
    el.toast.className = 'toast' + (ok ? ' ok' : '');
    el.toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.toast.hidden = true; }, 2600);
  }

  /* ---------- actions ---------- */

  async function post(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body || {}),
    });
    const data = await res.json().catch(() => ({ ok: false, error: '服务无响应' }));
    if (!res.ok || !data.ok) throw new Error(data.error || ('请求失败 ' + res.status));
    return data;
  }

  function selectedCards() {
    const s = seat(HUMAN);
    return (s.hand || []).filter((c) => state.selected.has(key(c)));
  }

  async function play() {
    try {
      await post('/api/play', { cards: selectedCards() });
      state.selected.clear();
      setStatus('');
      await poll();
    } catch (err) {
      toast(err.message);
    }
  }

  async function pass() {
    try {
      await post('/api/pass');
      await poll();
    } catch (err) {
      toast(err.message);
    }
  }

  async function hint() {
    try {
      const data = await post('/api/hint');
      if (data.hint.cards && data.hint.cards.length) {
        state.selected = new Set(data.hint.cards.map(key));
        renderHand();
        renderControls();
        setStatus(data.hint.text);
      } else {
        state.selected.clear();
        renderHand();
        renderControls();
        setStatus(data.hint.text, true);
      }
    } catch (err) {
      toast(err.message);
    }
  }

  async function restart(bet, resetScores) {
    try {
      await post('/api/restart', { bet: bet || 0, resetScores: !!resetScores });
      resetForNewRound();
      await poll();
    } catch (err) {
      toast(err.message);
    }
  }

  /* ---------- wiring ---------- */

  el.playBtn.addEventListener('click', play);
  el.passBtn.addEventListener('click', pass);
  el.hintBtn.addEventListener('click', hint);
  el.nextBtn.addEventListener('click', () => restart(0, false));
  el.againBtn.addEventListener('click', () => restart(0, false));
  el.resetBtn.addEventListener('click', () => restart(Number(el.bet.value), true));
  el.bet.addEventListener('change', () => restart(Number(el.bet.value), false));
  el.rulesBtn.addEventListener('click', () => { el.rulesModal.hidden = false; });
  el.closeRules.addEventListener('click', () => { el.rulesModal.hidden = true; });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') { el.rulesModal.hidden = true; el.resultModal.hidden = true; }
    if (e.key === 'Enter' && !el.playBtn.disabled) play();
    if (e.key === ' ') { e.preventDefault(); if (!el.passBtn.disabled) pass(); }
  });

  poll();
  setInterval(poll, POLL_MS);
})();
