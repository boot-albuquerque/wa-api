package liveness

// resetAck is what the page answers when the reconnect was CALLED. It is a
// distinct string rather than a boolean so that an empty answer — which is what
// an interrupted evaluation produces — cannot read as success.
const resetAck = "wa-headless/socket-reset"

// resetScript calls Socket.reconnect().
//
// It resolves the socket the same way spa.SocketStateReadExpr does, including
// the same three fallbacks, because a reset that reached a different object than
// the one the postcondition reads would verify the wrong socket.
const resetScript = `(() => {
	try {
		const m = window.require('WAWebSocketModel');
		const sk = m && (m.Socket || m.default || m);
		if (!sk || typeof sk.reconnect !== 'function') { return 'no-reconnect'; }
		sk.reconnect();
		return '` + resetAck + `';
	} catch (e) { return 'threw'; }
})()`
