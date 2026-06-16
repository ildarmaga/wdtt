(function () {
  function PanelEventsClient(base) {
    this.basePath = base || (typeof basePath !== 'undefined' ? basePath : '/');
    this.isConnected = false;
    this._handlers = Object.create(null);
    this._es = null;
  }

  PanelEventsClient.prototype.on = function (event, fn) {
    if (!this._handlers[event]) {
      this._handlers[event] = [];
    }
    this._handlers[event].push(fn);
  };

  PanelEventsClient.prototype._emit = function (event, payload) {
    var list = this._handlers[event] || [];
    for (var i = 0; i < list.length; i++) {
      try {
        list[i](payload);
      } catch (e) {
        console.error('wsClient handler', e);
      }
    }
  };

  PanelEventsClient.prototype.connect = function () {
    var self = this;
    if (this._es) {
      return;
    }
    if (typeof EventSource === 'undefined') {
      return;
    }
    var url = this.basePath + 'panel/api/server/events';
    this._es = new EventSource(url, { withCredentials: true });
    this._es.onopen = function () {
      self.isConnected = true;
    };
    this._es.onerror = function () {
      self.isConnected = false;
    };
    this._es.onmessage = function (ev) {
      try {
        var msg = JSON.parse(ev.data);
        if (msg && msg.type) {
          self._emit(msg.type, msg.obj);
        }
      } catch (e) {
        console.error('wsClient parse', e);
      }
    };
  };

  PanelEventsClient.prototype.disconnect = function () {
    if (this._es) {
      this._es.close();
      this._es = null;
    }
    this.isConnected = false;
  };

  window.wsClient = new PanelEventsClient(typeof basePath !== 'undefined' ? basePath : '/');
})();
