/* Sunshine 只读观测后台 · 虚拟智能体（Canvas 2D）
 * 单文件，无 three.js、无 WebGL、无 CDN。
 * 诚实规则：环境层（呼吸、慢轨道）只表示“系统活着在监听”；
 * 事件层（脉冲、闪烁、点亮）只由真实事件与真实状态触发。
 * native 模式下没有 worker 事件记录，节点一律显示“待命”，绝不假装在跑。
 */
(function () {
  'use strict';

  var ROLES = [
    { key: 'implement', label: 'implement' },
    { key: 'check', label: 'check' },
    { key: 'research', label: 'research' },
    { key: 'prototype', label: 'prototype' },
    { key: 'main', label: '主会话' }
  ];
  var STAGES = ['实现', '检查', '回写规范', '提交', '归档'];
  var MAX_PARTICLES = 120;

  function roleOfWorkerName(name) {
    var n = String(name || '').toLowerCase();
    if (n.indexOf('implement') !== -1) return 'implement';
    if (n.indexOf('check') !== -1) return 'check';
    if (n.indexOf('research') !== -1) return 'research';
    if (n.indexOf('prototype') !== -1) return 'prototype';
    if (n.indexOf('main') !== -1) return 'main';
    return null;
  }

  function workerStateOf(w) {
    var lc = String(w.Lifecycle || w.lifecycle || '').toLowerCase();
    var act = String(w.Activity || w.activity || '').toLowerCase();
    var exit = w.ExitCode;
    if ((exit !== undefined && exit !== null && exit !== 0) || lc.indexOf('error') !== -1 || lc.indexOf('fail') !== -1) return 'error';
    if (lc === 'running' || lc === 'active' || act === 'running' || act === 'in_turn' || act === 'working') return 'running';
    if (lc === 'idle' || lc === 'waiting' || act === 'idle' || act === 'waiting') return 'idle';
    if (lc.indexOf('done') !== -1 || lc.indexOf('complete') !== -1 || lc.indexOf('exit') !== -1 || lc.indexOf('terminat') !== -1) return 'done';
    return 'idle';
  }

  // 任务 status → 五轮流程线点亮范围；不谎称能区分 2.1 与 2.2
  function stageRange(status) {
    switch (String(status || '')) {
      case 'planning': return { pre: true, lit: [] };            // 需求/设计段
      case 'in_progress': return { pre: false, lit: [0, 1] };    // 开发段：实现/检查
      case 'review': case 'completed': return { pre: false, lit: [2, 3] }; // 收尾段：回写规范/提交
      case 'done': case 'archived': return { pre: false, lit: [0, 1, 2, 3, 4] };
      default: return { pre: false, lit: [] };
    }
  }

  function relAgo(nowMs, ts) {
    if (ts === null || ts === undefined || ts === '') return '—';
    var t = (typeof ts === 'number') ? (ts < 1e12 ? ts * 1000 : ts) : new Date(ts).getTime();
    if (isNaN(t)) return '—';
    var s = Math.max(0, Math.floor((nowMs - t) / 1000));
    if (s < 5) return '刚刚';
    if (s < 60) return s + ' 秒前';
    var m = Math.floor(s / 60);
    if (m < 60) return m + ' 分钟前';
    return Math.floor(m / 60) + ' 小时前';
  }

  window.SunshineHero = (function () {
    var canvas = null;
    var ctx = null;
    var onFallback = null;
    var rafId = null;
    var running = false;
    var visible = true;
    var reducedMotion = false;
    var width = 0;
    var height = 0;
    var dpr = 1;

    var state = {
      workers: [],
      channelMode: false,
      activityRate30s: 0,
      currentTaskStatus: '',
      currentTaskTitle: '',
      runningWorkers: 0,
      todayEvents: 0,
      lastActivityTs: null,
      nowMs: Date.now()
    };

    var pulses = [];      // 事件脉冲：{x0,y0,x1,y1,t,role}
    var ripples = [];     // 核心脉冲波纹：{r, alpha}
    var trails = [];      // 拖尾粒子：{x,y,vx,vy,life,role}
    var flashes = {};     // 节点到达闪亮：role -> 直到某毫秒
    var lastRippleAt = 0;
    var frameMsEma = 16;
    var particleBudget = MAX_PARTICLES;
    var t0 = Date.now();

    function resize() {
      if (!canvas) return;
      var rect = canvas.getBoundingClientRect();
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      width = Math.max(320, rect.width || canvas.clientWidth || 800);
      height = 340;
      canvas.width = Math.round(width * dpr);
      canvas.height = Math.round(height * dpr);
      canvas.style.height = height + 'px';
    }

    function nodePos(i, t) {
      // 五个 worker 节点分布在核心周围的椭圆轨道上，缓慢漂移
      var cx = width / 2;
      var cy = height * 0.42;
      var rx = Math.min(width * 0.36, 420);
      var ry = height * 0.26;
      var base = (-Math.PI / 2) + (i * (Math.PI * 2 / ROLES.length));
      var a = base + Math.sin(t / 9000 + i * 1.7) * 0.06;
      return { x: cx + Math.cos(a) * rx, y: cy + Math.sin(a) * ry, angle: a };
    }

    function corePos() {
      return { x: width / 2, y: height * 0.42 };
    }

    function roleState(role) {
      if (!state.channelMode) return 'standby'; // native 模式：无 worker 事件记录，一律待命
      var found = null;
      state.workers.forEach(function (w) {
        var name = w.name || w.Name || w.id || w.ID || '';
        if (roleOfWorkerName(name) === role) found = w;
      });
      if (role === 'main' && !found) {
        // 主会话：取未被归类的 worker
        state.workers.forEach(function (w) {
          var name = w.name || w.Name || w.id || w.ID || '';
          if (!found && roleOfWorkerName(name) === null) found = w;
        });
      }
      if (!found) return 'absent';
      return workerStateOf(found);
    }

    function drawStatic(t) {
      var core = corePos();
      var intensity = Math.min(1, state.activityRate30s / 10); // 最近 30 秒活动强度
      var breathe = 0.5 + 0.5 * Math.sin(t / 1200);

      ctx.clearRect(0, 0, width, height);
      ctx.save();
      ctx.scale(dpr, dpr);

      // 背景星点（静态装饰，不代表数据）
      ctx.fillStyle = 'rgba(143,163,191,0.35)';
      for (var s = 0; s < 40; s++) {
        var sx = (s * 137.5) % width;
        var sy = (s * 89.3) % (height * 0.7);
        ctx.fillRect(sx, sy, 1.5, 1.5);
      }

      // 轨道
      ctx.strokeStyle = 'rgba(59,130,246,0.42)';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.ellipse(core.x, core.y, Math.min(width * 0.36, 420), height * 0.26, 0, 0, Math.PI * 2);
      ctx.stroke();

      // 核心：同心圆环 + 脉冲波纹
      var glow = 24 + intensity * 30 + breathe * 6;
      ctx.shadowColor = '#22d3ee';
      ctx.shadowBlur = glow;
      ctx.strokeStyle = 'rgba(34,211,238,' + (0.7 + intensity * 0.3) + ')';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.arc(core.x, core.y, 34 + breathe * 4, 0, Math.PI * 2);
      ctx.stroke();
      ctx.shadowBlur = 0;
      ctx.strokeStyle = 'rgba(59,130,246,0.5)';
      ctx.beginPath();
      ctx.arc(core.x, core.y, 52 + breathe * 5, 0, Math.PI * 2);
      ctx.stroke();
      ctx.fillStyle = 'rgba(34,211,238,0.9)';
      ctx.beginPath();
      ctx.arc(core.x, core.y, 10 + breathe * 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = '#d7e3f4';
      ctx.font = '12px ui-monospace, Menlo, monospace';
      ctx.textAlign = 'center';
      ctx.fillText('AGENT', core.x, core.y + 4);

      // 波纹
      ripples.forEach(function (r) {
        ctx.strokeStyle = 'rgba(34,211,238,' + r.alpha + ')';
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.arc(core.x, core.y, r.r, 0, Math.PI * 2);
        ctx.stroke();
      });

      // worker 节点
      ROLES.forEach(function (role, i) {
        var p = nodePos(i, t);
        var st = roleState(role.key);
        var flashUntil = flashes[role.key] || 0;
        var flashing = Date.now() < flashUntil;
        var color = '#5b6d88';
        var bright = false;
        var speed = 1;
        if (st === 'running') { color = '#22c55e'; bright = true; speed = 2.4; }
        else if (st === 'idle') { color = '#f59e0b'; }
        else if (st === 'error') { color = '#ef4444'; bright = true; }
        else if (st === 'done') { color = '#3b82f6'; }
        else if (st === 'standby') { color = '#8fa3bf'; }
        if (flashing) { color = '#22d3ee'; bright = true; }
        if (st === 'absent') { color = '#2a3d5f'; }

        // 连线
        ctx.strokeStyle = bright ? 'rgba(34,211,238,0.45)' : 'rgba(59,130,246,0.18)';
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(core.x, core.y);
        ctx.lineTo(p.x, p.y);
        ctx.stroke();

        if (bright) { ctx.shadowColor = color; ctx.shadowBlur = 14; }
        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.arc(p.x, p.y, flashing ? 8 : 6, 0, Math.PI * 2);
        ctx.fill();
        ctx.shadowBlur = 0;
        // 待命节点加一圈描边：待机时也看得见它在场，而不是缺了一块
        if (st === 'standby') {
          ctx.strokeStyle = 'rgba(143,163,191,0.55)';
          ctx.lineWidth = 1;
          ctx.beginPath();
          ctx.arc(p.x, p.y, 10, 0, Math.PI * 2);
          ctx.stroke();
        }

        ctx.fillStyle = st === 'absent' ? '#5b6d88' : '#d7e3f4';
        ctx.font = '11px ui-monospace, Menlo, monospace';
        ctx.textAlign = 'center';
        var label = role.label + (st === 'standby' ? ' · 待命' : st === 'running' ? ' · 运行中' : st === 'error' ? ' · 出错' : st === 'done' ? ' · 已结束' : st === 'idle' ? ' · 空闲' : st === 'absent' ? ' · 无' : '');
        ctx.fillText(label, p.x, p.y + 22);
        void speed;
      });

      // 事件脉冲光点
      pulses.forEach(function (pu) {
        var x = pu.x0 + (pu.x1 - pu.x0) * pu.t;
        var y = pu.y0 + (pu.y1 - pu.y0) * pu.t;
        ctx.shadowColor = '#22d3ee';
        ctx.shadowBlur = 12;
        ctx.fillStyle = '#22d3ee';
        ctx.beginPath();
        ctx.arc(x, y, 4, 0, Math.PI * 2);
        ctx.fill();
        ctx.shadowBlur = 0;
      });

      // 拖尾粒子
      trails.forEach(function (pt) {
        ctx.fillStyle = 'rgba(34,211,238,' + Math.max(0, pt.life) + ')';
        ctx.beginPath();
        ctx.arc(pt.x, pt.y, 1.8, 0, Math.PI * 2);
        ctx.fill();
      });

      drawFlowLine(t);
      drawHud(t);
      ctx.restore();
    }

    function drawFlowLine(t) {
      var y = height - 44;
      var x0 = Math.max(30, width * 0.08);
      var x1 = width - x0;
      var range = stageRange(state.currentTaskStatus);
      var step = (x1 - x0) / (STAGES.length - 1);

      ctx.strokeStyle = 'rgba(59,130,246,0.35)';
      ctx.lineWidth = 2;
      ctx.beginPath();
      ctx.moveTo(x0, y);
      ctx.lineTo(x1, y);
      ctx.stroke();

      // 需求/设计前置段（planning 时点亮）
      if (range.pre) {
        ctx.fillStyle = '#22d3ee';
        ctx.font = 'bold 11px ui-monospace, Menlo, monospace';
        ctx.textAlign = 'left';
        ctx.fillText('● 需求/设计（规划中）', x0, y - 18);
      }

      STAGES.forEach(function (label, i) {
        var x = x0 + step * i;
        var lit = range.lit.indexOf(i) !== -1;
        var done = range.lit.length === STAGES.length;
        ctx.shadowColor = lit ? '#22d3ee' : 'transparent';
        ctx.shadowBlur = lit ? 10 : 0;
        ctx.fillStyle = lit ? '#22d3ee' : '#2a3d5f';
        ctx.beginPath();
        ctx.arc(x, y, lit ? 7 : 5, 0, Math.PI * 2);
        ctx.fill();
        ctx.shadowBlur = 0;
        if (done) {
          ctx.fillStyle = '#070b12';
          ctx.font = 'bold 9px sans-serif';
          ctx.textAlign = 'center';
          ctx.fillText('✓', x, y + 3);
        }
        ctx.fillStyle = lit ? '#d7e3f4' : '#8fa3bf';
        ctx.font = '11px sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText(label, x, y + 22);
      });
    }

    function drawHud(t) {
      void t;
      var items = [
        '运行中 worker ' + state.runningWorkers,
        '今日活动 ' + state.todayEvents,
        '最后活动 ' + relAgo(state.nowMs, state.lastActivityTs),
        '当前任务 ' + (state.currentTaskTitle || '—')
      ];
      ctx.font = '11px ui-monospace, Menlo, monospace';
      ctx.textAlign = 'left';
      var x = 12;
      items.forEach(function (it) {
        ctx.fillStyle = '#8fa3bf';
        ctx.fillText(it, x, 18);
        x += ctx.measureText(it).width + 28;
      });
    }

    function spawnTrail(role, x, y) {
      if (trails.length >= particleBudget) return;
      trails.push({
        x: x, y: y,
        vx: (Math.random() - 0.5) * 0.6,
        vy: (Math.random() - 0.5) * 0.6 - 0.2,
        life: 1, role: role
      });
    }

    function frame(now) {
      if (!running) return;
      var dt = now - (frame.last || now);
      frame.last = now;
      frameMsEma = frameMsEma * 0.9 + Math.min(dt, 100) * 0.1;
      // 帧率低时自动降粒子
      if (frameMsEma > 40) particleBudget = Math.max(30, particleBudget - 10);
      else if (frameMsEma < 20 && particleBudget < MAX_PARTICLES) particleBudget = Math.min(MAX_PARTICLES, particleBudget + 2);

      var t = now - t0;
      // 核心波纹：活动越强越密
      var intensity = Math.min(1, state.activityRate30s / 10);
      var interval = 2600 - intensity * 1800;
      if (now - lastRippleAt > interval) {
        ripples.push({ r: 44, alpha: 0.6 });
        lastRippleAt = now;
      }
      ripples.forEach(function (r) { r.r += 0.9 + intensity; r.alpha -= 0.006; });
      ripples = ripples.filter(function (r) { return r.alpha > 0 && r.r < 160; });

      // 脉冲推进
      pulses.forEach(function (pu) { pu.t += 0.02; });
      pulses = pulses.filter(function (pu) {
        if (pu.t >= 1) {
          flashes[pu.role] = Date.now() + 900;
          return false;
        }
        return true;
      });

      // 拖尾：运行中节点才产粒子
      ROLES.forEach(function (role, i) {
        if (roleState(role.key) === 'running' && Math.random() < 0.5) {
          var p = nodePos(i, t);
          spawnTrail(role.key, p.x, p.y);
        }
      });
      trails.forEach(function (pt) { pt.x += pt.vx; pt.y += pt.vy; pt.life -= 0.015; });
      trails = trails.filter(function (pt) { return pt.life > 0; });

      drawStatic(t);
      rafId = requestAnimationFrame(frame);
    }

    function loop(now) { frame(now); }

    return {
      init: function (canvasEl, fallbackCb) {
        canvas = canvasEl;
        onFallback = fallbackCb || null;
        reducedMotion = !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
        try {
          ctx = canvas.getContext('2d');
        } catch (e) {
          ctx = null;
        }
        if (!ctx) {
          if (onFallback) onFallback();
          return false;
        }
        resize();
        window.addEventListener('resize', resize);
        document.addEventListener('visibilitychange', function () {
          visible = !document.hidden;
          if (visible) { if (!running) { running = true; rafId = requestAnimationFrame(loop); } }
          else { running = false; if (rafId) cancelAnimationFrame(rafId); }
        });
        if (reducedMotion) {
          // 只画一帧静态画面
          drawStatic(Date.now() - t0);
          return true;
        }
        running = true;
        rafId = requestAnimationFrame(loop);
        return true;
      },

      setState: function (s) {
        if (!s) return;
        Object.keys(s).forEach(function (k) { state[k] = s[k]; });
        if (reducedMotion && ctx) drawStatic(Date.now() - t0);
      },

      // 新活动到达时由 app.js 调用：光点从核心飞向对应节点
      pulse: function (role) {
        if (!ctx || reducedMotion) return;
        var idx = 0;
        ROLES.forEach(function (r, i) { if (r.key === role) idx = i; });
        var core = corePos();
        var p = nodePos(idx, Date.now() - t0);
        if (pulses.length >= particleBudget) return;
        pulses.push({ x0: core.x, y0: core.y, x1: p.x, y1: p.y, t: 0, role: role });
      }
    };
  })();
})();
