/* Sunshine 只读观测后台 · 数据层（Alpine.js）
 * 全部 GET 轮询；轮询间隔取 /api/status 的 poll_interval_sec，默认 3 秒。
 * 无数据时显示空状态，绝不编造数字、事件或任务名。
 */
(function () {
  'use strict';

  function toMs(ts) {
    if (ts === null || ts === undefined || ts === '') return null;
    if (typeof ts === 'number') return ts < 1e12 ? ts * 1000 : ts;
    var d = new Date(ts);
    return isNaN(d.getTime()) ? null : d.getTime();
  }

  function relTimeMs(nowMs, tsMs) {
    if (tsMs === null) return '—';
    var diff = Math.max(0, Math.floor((nowMs - tsMs) / 1000));
    if (diff < 5) return '刚刚';
    if (diff < 60) return diff + ' 秒前';
    var m = Math.floor(diff / 60);
    if (m < 60) return m + ' 分钟前';
    var h = Math.floor(m / 60);
    if (h < 24) return h + ' 小时前';
    var d = Math.floor(h / 24);
    return d + ' 天前';
  }

  document.addEventListener('alpine:init', function () {
    Alpine.data('serveApp', function () {
      return {
        /* ---- 顶栏 / 状态 ---- */
        status: null,
        versionInfo: {},
        pollIntervalSec: 3,
        lastPollAt: null,
        pollStale: false,
        nowMs: Date.now(),
        clockText: '',
        isFullscreen: false,
        errors: {},

        /* ---- 任务 ---- */
        tasks: [],
        currentTaskInfo: null,
        selectedTaskId: null,
        taskDetail: null,
        taskCommits: null,
        archiveMonths: [],
        archiveMonth: '',
        archive: null,

        /* ---- channel / 事件 / worker ---- */
        channels: [],
        selectedChannel: null,
        events: [],
        eventMaxSeq: 0,
        eventFilterKind: '',
        eventFilterFrom: '',
        rawOpen: {},
        workers: [],
        selectedWorker: null,
        workerLog: [],
        logLines: 100,
        logFollow: true,
        logUserScrolledUp: false,

        /* ---- forum ---- */
        threads: [],
        selectedThread: null,
        threadDetail: null,

        /* ---- 记忆 ---- */
        memSessions: [],
        memProjects: [],
        memPlatform: '',

        /* ---- 健康 / 设计 / journal ---- */
        health: null,
        design: null,
        journal: null,
        selectedJournal: null,
        journalByFile: {},

        /* ---- 活动流 ---- */
        activity: { entries: [] },
        lastSeenActivityTs: null,
        lastSeenActivityCount: 0,

        /* ---- 变化高亮 ---- */
        flashKeys: {},
        prevTaskStatus: {},
        prevChannelEvents: {},

        heroFallback: false,
        pollTimer: null,
        clockTimer: null,

        /* ============ 启动 ============ */
        init: function () {
          var self = this;
          this.updateClock();
          this.clockTimer = setInterval(function () {
            self.nowMs = Date.now();
            self.updateClock();
          }, 1000);
          document.addEventListener('fullscreenchange', function () {
            self.isFullscreen = !!document.fullscreenElement;
          });
          // 先拉一次 status 拿到轮询间隔，再启动轮询
          this.fetchStatus().then(function () {
            self.pollAll();
            self.startPolling();
          });
          // hero 画布初始化（hero.js 提供 window.SunshineHero）
          this.initHero();
        },

        startPolling: function () {
          var self = this;
          if (this.pollTimer) clearInterval(this.pollTimer);
          this.pollTimer = setInterval(function () { self.pollAll(); }, this.pollIntervalSec * 1000);
        },

        updateClock: function () {
          var d = new Date(this.nowMs);
          function p(n) { return (n < 10 ? '0' : '') + n; }
          this.clockText = p(d.getHours()) + ':' + p(d.getMinutes()) + ':' + p(d.getSeconds());
        },

        toggleFullscreen: function () {
          if (document.fullscreenElement) {
            document.exitFullscreen();
          } else if (document.documentElement.requestFullscreen) {
            document.documentElement.requestFullscreen();
          }
        },

        /* ============ 通用取数 ============ */
        fetchJson: function (url) {
          return fetch(url, { headers: { 'Accept': 'application/json' } }).then(function (r) {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.json();
          });
        },

        fetchStatus: function () {
          var self = this;
          return this.fetchJson('/api/status').then(function (data) {
            self.status = data;
            if (data && data.poll_interval_sec) {
              var iv = parseInt(data.poll_interval_sec, 10);
              if (iv >= 1 && iv <= 30 && iv !== self.pollIntervalSec) {
                self.pollIntervalSec = iv;
                self.startPolling();
              }
            }
          }).catch(function (e) {
            self.errors.status = '状态接口失败：' + e.message;
          });
        },

        pollAll: function () {
          var self = this;
          this.fetchStatus();
          this.loadTasks();
          this.loadCurrentTask();
          this.loadChannels();
          this.loadActivity();
          this.loadMemSessions();
          this.loadMemProjects();
          this.loadHealth();
          this.loadDesign();
          this.loadJournal();
          if (this.selectedChannel) {
            this.loadEvents(false);
            this.loadWorkers();
            this.loadThreads();
            if (this.selectedWorker) this.loadWorkerLog();
            if (this.selectedThread) this.loadThreadDetail();
          }
          if (this.selectedTaskId) this.refreshTaskDetail();
          this.lastPollAt = Date.now();
          this.pollStale = false;
          setTimeout(function () { self.pollStale = (Date.now() - self.lastPollAt) > (self.pollIntervalSec * 1000 + 5000); }, (self.pollIntervalSec * 1000 + 5000));
          this.pushHeroState();
        },

        /* ============ 相对时间 ============ */
        relTime: function (ts) {
          return relTimeMs(this.nowMs, toMs(ts));
        },

        /* ============ 变化高亮 ============ */
        flash: function (key) {
          var self = this;
          this.flashKeys[key] = true;
          setTimeout(function () { delete self.flashKeys[key]; }, 1700);
        },
        flashKey: function (key) { return !!this.flashKeys[key]; },

        /* ============ 任务 ============ */
        taskKey: function (t) { return t.dir || t.id || t.title || ''; },

        loadTasks: function () {
          var self = this;
          this.fetchJson('/api/tasks').then(function (data) {
            var list = Array.isArray(data) ? data : (data.tasks || []);
            list.forEach(function (t) {
              var k = self.taskKey(t);
              var prev = self.prevTaskStatus[k];
              if (prev !== undefined && prev !== t.status) self.flash('task:' + k);
              self.prevTaskStatus[k] = t.status;
            });
            self.tasks = list;
          }).catch(function (e) { self.errors.tasks = '任务列表失败：' + e.message; });
        },

        topTasks: function () {
          var self = this;
          return this.tasks.filter(function (t) { return !t.parent; });
        },
        childTasks: function (t) {
          var self = this;
          var key = this.taskKey(t);
          return this.tasks.filter(function (c) {
            return c.parent && (c.parent === key || c.parent === t.id || c.parent === t.dir);
          });
        },

        loadCurrentTask: function () {
          var self = this;
          this.fetchJson('/api/tasks/current').then(function (data) {
            self.currentTaskInfo = data;
          }).catch(function () { /* 当前任务缺失不报错，只留空 */ });
        },

        currentTaskTitle: function () {
          if (this.currentTaskInfo && this.currentTaskInfo.current_task) {
            var ct = this.currentTaskInfo.current_task;
            return (typeof ct === 'string') ? ct : (ct.title || ct.id || ct.dir || '—');
          }
          if (this.status && this.status.current_task) {
            var s = this.status.current_task;
            return (typeof s === 'string') ? s : (s.title || s.id || s.dir || '—');
          }
          return '暂无当前任务';
        },
        currentTaskLine: function () {
          if (!this.currentTaskInfo) return '';
          var parts = [];
          if (this.currentTaskInfo.source) parts.push('来源：' + this.currentTaskInfo.source);
          if (this.currentTaskInfo.stale) parts.push('可能已过期');
          return parts.join(' · ');
        },

        selectTask: function (t) {
          this.selectedTaskId = this.taskKey(t);
          this.refreshTaskDetail();
        },
        closeTaskDetail: function () {
          this.selectedTaskId = null;
          this.taskDetail = null;
          this.taskCommits = null;
        },
        refreshTaskDetail: function () {
          var self = this;
          if (!this.selectedTaskId) return;
          this.fetchJson('/api/tasks/' + encodeURIComponent(this.selectedTaskId)).then(function (data) {
            self.taskDetail = data;
            self.errors.taskDetail = '';
          }).catch(function (e) { self.errors.taskDetail = '任务详情失败：' + e.message; });
          this.fetchJson('/api/tasks/' + encodeURIComponent(this.selectedTaskId) + '/commits').then(function (data) {
            self.taskCommits = Array.isArray(data) ? data : (data.commits || []);
          }).catch(function () { self.taskCommits = []; });
        },

        detailFields: function () {
          if (!this.taskDetail) return [];
          var skip = { prd_md: 1, implement: 1, check: 1, subtasks: 1 };
          var out = [];
          Object.keys(this.taskDetail).forEach(function (k) {
            if (!skip[k]) out.push([k, this.taskDetail[k]]);
          }, this);
          return out;
        },
        fieldText: function (v) {
          if (v === null || v === undefined) return '—';
          if (Array.isArray(v)) return v.length ? v.join('、') : '—';
          if (typeof v === 'object') return JSON.stringify(v);
          return String(v);
        },

        statusBadgeClass: function (status) {
          switch (status) {
            case 'in_progress': return 'green';
            case 'review': return 'amber';
            case 'completed': case 'done': return 'blue';
            case 'archived': return 'gray';
            case 'planning': return 'cyan';
            default: return '';
          }
        },

        loadArchive: function () {
          var self = this;
          var url = '/api/tasks/archive' + (this.archiveMonth ? ('?month=' + encodeURIComponent(this.archiveMonth)) : '');
          this.fetchJson(url).then(function (data) {
            if (Array.isArray(data)) { self.archive = data; return; }
            if (data && Array.isArray(data.tasks)) { self.archive = data.tasks; return; }
            if (data && data.months) {
              self.archiveMonths = data.months.map(function (m) { return m.month || m; });
              self.archive = [];
              return;
            }
            self.archive = [];
          }).catch(function () { self.archive = []; });
        },

        /* ============ channel / 事件 ============ */
        loadChannels: function () {
          var self = this;
          this.fetchJson('/api/channels').then(function (data) {
            var list = Array.isArray(data) ? data : (data.channels || []);
            list.forEach(function (c) {
              var prev = self.prevChannelEvents[c.name];
              if (prev !== undefined && c.totalEvents !== undefined && c.totalEvents !== prev) {
                self.flash('channel:' + c.name);
              }
              if (c.totalEvents !== undefined) self.prevChannelEvents[c.name] = c.totalEvents;
            });
            self.channels = list;
            if (!self.selectedChannel && list.length) self.selectChannel(list[0].name);
          }).catch(function (e) { self.errors.channels = 'Channel 列表失败：' + e.message; });
        },

        totalEvents: function () {
          var n = 0, any = false;
          this.channels.forEach(function (c) {
            if (typeof c.totalEvents === 'number') { n += c.totalEvents; any = true; }
          });
          return any ? n : '—';
        },

        selectChannel: function (name) {
          if (this.selectedChannel === name && this.events.length) return;
          this.selectedChannel = name;
          this.events = [];
          this.eventMaxSeq = 0;
          this.workers = [];
          this.selectedWorker = null;
          this.workerLog = [];
          this.threads = [];
          this.selectedThread = null;
          this.threadDetail = null;
          this.loadEvents(true);
          this.loadWorkers();
          this.loadThreads();
        },

        loadEvents: function (full) {
          var self = this;
          if (!this.selectedChannel) return;
          var url = '/api/channels/' + encodeURIComponent(this.selectedChannel) + '/events';
          if (full || !this.eventMaxSeq) {
            url += '?last=100';
          } else {
            url += '?since=' + this.eventMaxSeq;
          }
          this.fetchJson(url).then(function (data) {
            var list = Array.isArray(data) ? data : (data.events || []);
            list.forEach(function (e) {
              if (typeof e.seq === 'number' && e.seq > self.eventMaxSeq) self.eventMaxSeq = e.seq;
            });
            if (full || !self.eventMaxSeq) {
              self.events = list;
            } else {
              var merged = self.events.concat(list);
              if (merged.length > 300) merged = merged.slice(merged.length - 300);
              self.events = merged;
            }
            list.forEach(function (e) { self.flash('ev:' + e.seq); });
            self.errors.events = '';
          }).catch(function (e) { self.errors.events = '事件流失败：' + e.message; });
        },

        eventKinds: function () {
          var seen = {};
          this.events.forEach(function (e) { if (e.kind) seen[e.kind] = true; });
          return Object.keys(seen).sort();
        },

        filteredEvents: function () {
          var kind = this.eventFilterKind;
          var from = (this.eventFilterFrom || '').trim();
          return this.events.filter(function (e) {
            if (kind && e.kind !== kind) return false;
            if (from) {
              var author = e.author || e.from || e.actor || '';
              if (String(author).indexOf(from) === -1) return false;
            }
            return true;
          });
        },

        eventSummary: function (e) {
          if (e.summary || e.text || e.message) return e.summary || e.text || e.message;
          var parts = [];
          if (e.author || e.from) parts.push(e.author || e.from);
          if (e.target) parts.push('→ ' + e.target);
          if (e.thread) parts.push('[' + e.thread + ']');
          if (e.detail) parts.push(typeof e.detail === 'string' ? e.detail : JSON.stringify(e.detail));
          return parts.length ? parts.join(' ') : (e.kind || '');
        },

        toggleRaw: function (seq) {
          this.rawOpen[seq] = !this.rawOpen[seq];
        },

        /* ============ worker ============ */
        workerName: function (w) { return w.name || w.Name || w.id || w.ID || 'worker'; },

        loadWorkers: function () {
          var self = this;
          if (!this.selectedChannel) return;
          this.fetchJson('/api/channels/' + encodeURIComponent(this.selectedChannel) + '/workers').then(function (data) {
            self.workers = Array.isArray(data) ? data : (data.workers || []);
            self.errors.workers = '';
          }).catch(function (e) { self.errors.workers = 'Worker 状态失败：' + e.message; });
        },

        runningWorkers: function () {
          var n = 0;
          this.workers.forEach(function (w) {
            var lc = String(w.Lifecycle || w.lifecycle || '').toLowerCase();
            var act = String(w.Activity || w.activity || '').toLowerCase();
            if (lc === 'running' || lc === 'active' || act === 'running' || act === 'in_turn' || act === 'working') n++;
          });
          return this.workers.length ? n : '—';
        },

        workerLifecycleText: function (w) {
          return w.Lifecycle || w.lifecycle || w.status || '—';
        },
        workerActivityText: function (w) {
          return w.Activity || w.activity || '';
        },
        workerLampClass: function (w) {
          var lc = String(w.Lifecycle || w.lifecycle || '').toLowerCase();
          var act = String(w.Activity || w.activity || '').toLowerCase();
          if (w.ExitCode !== undefined && w.ExitCode !== null && w.ExitCode !== 0) return 'red';
          if (lc.indexOf('error') !== -1 || lc.indexOf('fail') !== -1) return 'red';
          if (lc === 'running' || lc === 'active' || act === 'running' || act === 'in_turn' || act === 'working') return 'green';
          if (lc === 'idle' || lc === 'waiting' || act === 'idle') return 'amber';
          return 'gray';
        },

        selectWorker: function (w) {
          this.selectedWorker = this.workerName(w);
          this.workerLog = [];
          this.logUserScrolledUp = false;
          this.loadWorkerLog();
        },

        loadWorkerLog: function () {
          var self = this;
          if (!this.selectedChannel || !this.selectedWorker) return;
          var url = '/api/channels/' + encodeURIComponent(this.selectedChannel) +
            '/workers/' + encodeURIComponent(this.selectedWorker) + '/log?lines=' + this.logLines;
          this.fetchJson(url).then(function (data) {
            self.workerLog = Array.isArray(data) ? data : (data.lines || []);
            self.errors.workerLog = '';
            self.maybeScrollLog();
          }).catch(function (e) { self.errors.workerLog = '日志尾部失败：' + e.message; });
        },

        onLogScroll: function () {
          var box = this.$refs.logBox;
          if (!box) return;
          this.logUserScrolledUp = (box.scrollHeight - box.scrollTop - box.clientHeight) > 40;
        },
        maybeScrollLog: function () {
          var self = this;
          this.$nextTick(function () {
            var box = self.$refs.logBox;
            if (box && self.logFollow && !self.logUserScrolledUp) box.scrollTop = box.scrollHeight;
          });
        },

        /* ============ forum ============ */
        threadKey: function (t) { return t.id || t.thread || t.title || ''; },

        loadThreads: function () {
          var self = this;
          if (!this.selectedChannel) return;
          this.fetchJson('/api/channels/' + encodeURIComponent(this.selectedChannel) + '/threads').then(function (data) {
            self.threads = Array.isArray(data) ? data : (data.threads || []);
            self.errors.threads = '';
          }).catch(function (e) { self.errors.threads = 'Forum 列表失败：' + e.message; });
        },

        selectThread: function (t) {
          this.selectedThread = this.threadKey(t);
          this.loadThreadDetail();
        },
        loadThreadDetail: function () {
          var self = this;
          if (!this.selectedChannel || !this.selectedThread) return;
          this.fetchJson('/api/channels/' + encodeURIComponent(this.selectedChannel) + '/threads/' + encodeURIComponent(this.selectedThread)).then(function (data) {
            self.threadDetail = data;
          }).catch(function () { self.threadDetail = null; });
        },
        threadMessages: function () {
          if (!this.threadDetail) return [];
          if (Array.isArray(this.threadDetail)) return this.threadDetail;
          return this.threadDetail.messages || this.threadDetail.events || this.threadDetail.timeline || [];
        },

        /* ============ 记忆 ============ */
        memPlatforms: function () {
          var seen = {};
          this.memSessions.forEach(function (s) { if (s.Platform) seen[s.Platform] = true; });
          return Object.keys(seen).sort();
        },
        loadMemSessions: function () {
          var self = this;
          var url = '/api/mem/sessions' + (this.memPlatform ? ('?platform=' + encodeURIComponent(this.memPlatform)) : '');
          this.fetchJson(url).then(function (data) {
            self.memSessions = Array.isArray(data) ? data : (data.sessions || []);
            self.errors.mem = '';
          }).catch(function (e) { self.errors.mem = '会话列表失败：' + e.message; });
        },
        loadMemProjects: function () {
          var self = this;
          this.fetchJson('/api/mem/projects').then(function (data) {
            self.memProjects = Array.isArray(data) ? data : (data.projects || []);
          }).catch(function () { self.memProjects = []; });
        },

        /* ============ 健康 / 设计 / journal ============ */
        loadHealth: function () {
          var self = this;
          this.fetchJson('/api/health').then(function (data) {
            self.health = data;
            self.errors.health = '';
          }).catch(function (e) { self.errors.health = '项目健康失败：' + e.message; });
          this.fetchJson('/api/version').then(function (data) {
            self.versionInfo = data || {};
          }).catch(function () { /* 版本端点缺失时用 status 里的版本 */ });
        },

        healthBuckets: function () {
          if (!this.health) return [];
          var h = this.health;
          var drift = h.drift || h.template_drift || h.buckets || {};
          function num(v) { return (typeof v === 'number') ? v : '—'; }
          return [
            { label: '新增', value: num(drift.added ?? h.added), cls: '' },
            { label: '可自动更新', value: num(drift.updatable ?? h.updatable), cls: '' },
            { label: '你已修改', value: num(drift.modified ?? h.modified), cls: (drift.modified || h.modified) ? 'warn' : '' },
            { label: '未变更', value: num(drift.unchanged ?? h.unchanged), cls: 'ok' },
            { label: '你已删除', value: num(drift.deleted ?? h.deleted), cls: (drift.deleted || h.deleted) ? 'bad' : '' }
          ];
        },
        healthExtra: function () {
          if (!this.health) return [];
          var skip = { drift: 1, template_drift: 1, buckets: 1, added: 1, updatable: 1, modified: 1, unchanged: 1, deleted: 1, project_version: 1, cli_version: 1, latest_version: 1 };
          var out = [];
          Object.keys(this.health).forEach(function (k) {
            if (!skip[k]) out.push([k, this.health[k]]);
          }, this);
          return out;
        },

        loadDesign: function () {
          var self = this;
          this.fetchJson('/api/design').then(function (data) {
            self.design = data;
            self.errors.design = '';
          }).catch(function (e) { self.errors.design = '设计文档失败：' + e.message; });
        },

        designDocs: function () {
          if (!this.design) return [];
          var docs = this.design.docs || this.design.documents || this.design.files;
          if (Array.isArray(docs)) {
            return docs.map(function (d) {
              return {
                name: d.name || d.path || d.file || '',
                exists: !!(d.exists ?? d.present),
                countText: d.countText || d.counts || (d.lines !== undefined ? ('行数 ' + d.lines) : '—')
              };
            });
          }
          // 兜底：按三份已知文档名从顶层字段拼
          var names = ['需求列表.md', '概要设计.md', '原型.html'];
          var self = this;
          return names.map(function (n) {
            var key = n.replace(/[^a-zA-Z0-9]/g, '_');
            var v = self.design[n] || self.design[key];
            return { name: n, exists: !!(v && (v.exists ?? v.present ?? v)), countText: '—' };
          });
        },

        loadJournal: function () {
          var self = this;
          this.fetchJson('/api/journal').then(function (data) {
            self.journal = data || {};
            self.errors.journal = '';
            if (data && Array.isArray(data.files) && data.files.length && !self.selectedJournal) {
              // files 是 {name, path} 对象：存名字符串，按钮与缓存键保持一致
              var firstName = data.files[0].name || data.files[0];
              self.selectedJournal = firstName;
              if (data.content) self.journalByFile[firstName] = data.content;
            }
          }).catch(function (e) { self.errors.journal = 'Journal 失败：' + e.message; });
        },
        selectJournal: function (f) {
          var self = this;
          this.selectedJournal = f;
          if (this.journalByFile[f]) return;
          this.fetchJson('/api/journal?file=' + encodeURIComponent(f)).then(function (data) {
            if (data && data.content) self.journalByFile[f] = data.content;
          }).catch(function () { /* 后端若不支持按文件取，仍显示初始内容 */ });
        },
        journalContent: function () {
          if (this.selectedJournal && this.journalByFile[this.selectedJournal]) return this.journalByFile[this.selectedJournal];
          if (this.journal && this.journal.content) return this.journal.content;
          return '';
        },

        /* ============ 活动流 ============ */
        loadActivity: function () {
          var self = this;
          this.fetchJson('/api/activity').then(function (data) {
            var entries = Array.isArray(data) ? data : (data.entries || []);
            // 新条目：ts 大于上次见过的最新 ts，或条目数变多
            var maxTs = self.lastSeenActivityTs;
            var fresh = [];
            entries.forEach(function (e) {
              var t = toMs(e.ts);
              if (t !== null && (maxTs === null || t > maxTs)) fresh.push(e);
              if (t !== null && (maxTs === null || t > maxTs)) maxTs = t;
            });
            if (self.lastSeenActivityTs !== null && fresh.length) {
              fresh.forEach(function (e) {
                self.flash('act:' + e.ts + '|' + e.text);
                self.pulseHero(e);
              });
            }
            self.lastSeenActivityTs = maxTs;
            self.lastSeenActivityCount = entries.length;
            self.activity.entries = entries;
          }).catch(function (e) { self.errors.activity = '活动流失败：' + e.message; });
        },

        todayActivityCount: function () {
          var start = new Date();
          start.setHours(0, 0, 0, 0);
          var n = 0;
          this.activity.entries.forEach(function (e) {
            var t = toMs(e.ts);
            if (t !== null && t >= start.getTime()) n++;
          });
          return n;
        },
        lastActivityAgo: function () {
          if (!this.activity.entries.length) return '—';
          return this.relTime(this.activity.entries[0].ts);
        },

        /* ============ hero 联动 ============ */
        initHero: function () {
          var self = this;
          if (!window.SunshineHero) { this.heroFallback = true; return; }
          var canvas = document.getElementById('hero-canvas');
          if (!canvas) { this.heroFallback = true; return; }
          var ok = window.SunshineHero.init(canvas, function () { self.heroFallback = true; });
          if (!ok) this.heroFallback = true;
        },
        pulseHero: function (entry) {
          if (window.SunshineHero && window.SunshineHero.pulse) {
            window.SunshineHero.pulse(this.workerForEntry(entry));
          }
        },
        workerForEntry: function (entry) {
          var text = ((entry && entry.text) || '') + ' ' + ((entry && entry.kind) || '');
          var lower = text.toLowerCase();
          if (lower.indexOf('implement') !== -1) return 'implement';
          if (lower.indexOf('check') !== -1) return 'check';
          if (lower.indexOf('research') !== -1) return 'research';
          if (lower.indexOf('prototype') !== -1) return 'prototype';
          return 'main';
        },
        pushHeroState: function () {
          if (!window.SunshineHero || !window.SunshineHero.setState) return;
          var now = Date.now();
          var recent = 0;
          this.activity.entries.forEach(function (e) {
            var t = toMs(e.ts);
            if (t !== null && now - t <= 30000) recent++;
          });
          window.SunshineHero.setState({
            workers: this.workers,
            channelMode: !!(this.selectedChannel && this.events.length),
            activityRate30s: recent,
            currentTaskStatus: this.currentTaskStatus(),
            currentTaskTitle: this.currentTaskTitle(),
            runningWorkers: (typeof this.runningWorkers() === 'number') ? this.runningWorkers() : 0,
            todayEvents: this.todayActivityCount(),
            lastActivityTs: this.activity.entries.length ? this.activity.entries[0].ts : null,
            nowMs: now
          });
        },
        currentTaskStatus: function () {
          if (this.currentTaskInfo && this.currentTaskInfo.current_task && typeof this.currentTaskInfo.current_task === 'object') {
            return this.currentTaskInfo.current_task.status || '';
          }
          var title = this.currentTaskTitle();
          var found = null;
          this.tasks.forEach(function (t) {
            if ((t.title || t.id || t.dir) === title) found = t;
          });
          return found ? (found.status || '') : '';
        }
      };
    });
  });
})();
