// The desktop connection receives alerts from the backend. It reconnects one
// second after a connection ends, so a backend that restarts is found.
function connect(port, {open, alert}) {
  const dial = () => {
    const ws = new WebSocket(`ws://localhost:${port}/desktop`);
    ws.onopen = open;
    ws.onclose = () => setTimeout(dial, 1000);
    ws.onmessage = ({data}) => {
      const message = JSON.parse(data);
      if (message.type === 'alert') alert(message);
    };
  };
  dial();
}

module.exports = {connect};
