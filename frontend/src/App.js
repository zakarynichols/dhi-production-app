import React from 'react';
import './index.css';

function App() {
  const [bins, setBins] = React.useState([]);
  const [selectedBin, setSelectedBin] = React.useState(null);
  const [requests, setRequests] = React.useState([]);
  const [binName, setBinName] = React.useState('');
  const [loading, setLoading] = React.useState(false);
  const [copied, setCopied] = React.useState(null);

  React.useEffect(() => {
    loadBins();
  }, []);

  const loadBins = async () => {
    try {
      const res = await fetch('/api/bins');
      const data = await res.json();
      setBins(data);
    } catch (err) {
      console.error('Error loading bins:', err);
    }
  };

  const createBin = async (e) => {
    e.preventDefault();
    if (!binName.trim()) return;
    
    setLoading(true);
    try {
      const res = await fetch('/api/bins', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: binName })
      });
      const newBin = await res.json();
      setBins([newBin, ...bins]);
      setBinName('');
      setSelectedBin(newBin.id);
      loadRequests(newBin.id);
    } catch (err) {
      console.error('Error creating bin:', err);
    }
    setLoading(false);
  };

  const loadRequests = async (binId) => {
    try {
      const res = await fetch(`/api/bins/${binId}/requests`);
      const data = await res.json();
      setRequests(data);
    } catch (err) {
      console.error('Error loading requests:', err);
    }
  };

  const selectBin = async (binId) => {
    setSelectedBin(binId);
    loadRequests(binId);
  };

  const deleteBin = async (binId) => {
    try {
      await fetch(`/api/bins/${binId}`, { method: 'DELETE' });
      setBins(bins.filter(b => b.id !== binId));
      if (selectedBin === binId) {
        setSelectedBin(null);
        setRequests([]);
      }
    } catch (err) {
      console.error('Error deleting bin:', err);
    }
  };

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text);
    setCopied(text);
    setTimeout(() => setCopied(null), 2000);
  };

  const getBaseUrl = () => {
    return window.location.origin;
  };

  return (
    <div className="app">
      <header>
        <h1>Request Bin</h1>
        <p>Capture and inspect HTTP requests</p>
      </header>
      
      <main>
        <section className="create-bin">
          <h2>Create New Bin</h2>
          <form onSubmit={createBin}>
            <input
              type="text"
              placeholder="Bin name (optional)"
              value={binName}
              onChange={(e) => setBinName(e.target.value)}
            />
            <button type="submit" disabled={loading}>
              {loading ? 'Creating...' : 'Create Bin'}
            </button>
          </form>
        </section>

        <div className="bins-container">
          <section className="bins-list">
            <h2>Your Bins ({bins.length})</h2>
            {bins.length === 0 ? (
              <p className="empty">No bins yet. Create one above!</p>
            ) : (
              <ul>
                {bins.map(bin => (
                  <li 
                    key={bin.id} 
                    className={selectedBin === bin.id ? 'selected' : ''}
                    onClick={() => selectBin(bin.id)}
                  >
                    <span className="bin-id">{bin.id}</span>
                    {bin.name && <span className="bin-name">{bin.name}</span>}
                    <button 
                      className="delete-btn"
                      onClick={(e) => { e.stopPropagation(); deleteBin(bin.id); }}
                    >
                      Delete
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="bin-details">
            {selectedBin ? (
              <>
                <div className="bin-header">
                  <h2>Bin: {selectedBin}</h2>
                  <div className="bin-url">
                    <code>{getBaseUrl()}/{selectedBin}</code>
                    <button onClick={() => copyToClipboard(`${getBaseUrl()}/${selectedBin}`)}>
                      {copied ? 'Copied!' : 'Copy'}
                    </button>
                  </div>
                  <p className="hint">Send any HTTP request to the URL above to capture it</p>
                </div>

                <div className="requests-list">
                  <h3>Captured Requests ({requests.length})</h3>
                  {requests.length === 0 ? (
                    <p className="empty">No requests captured yet. Try:</p>
                  ) : (
                    <ul>
                      {requests.map(req => (
                        <li key={req.id}>
                          <div className="request-line">
                            <span className={`method ${req.method}`}>{req.method}</span>
                            <span className="path">{req.path}</span>
                            <span className="time">{new Date(req.created_at).toLocaleTimeString()}</span>
                          </div>
                          {req.body && (
                            <details>
                              <summary>Body</summary>
                              <pre>{req.body}</pre>
                            </details>
                          )}
                          <details>
                            <summary>Headers ({Object.keys(req.headers || {}).length})</summary>
                            <pre>{JSON.stringify(req.headers, null, 2)}</pre>
                          </details>
                          <div className="client-ip">From: {req.client_ip}</div>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </>
            ) : (
              <p className="empty">Select a bin to view captured requests</p>
            )}
          </section>
        </div>
      </main>
    </div>
  );
}

export default App;
