import React from 'react';
import './index.css';

interface Bin {
  id: string;
  name?: string;
  requestCount?: number;
}

interface Request {
  id: string;
  method: string;
  path: string;
  created_at: string;
  body?: string;
  headers?: Record<string, string>;
  client_ip?: string;
}

function App() {
  const [bins, setBins] = React.useState<Bin[]>([]);
  const [selectedBin, setSelectedBin] = React.useState<string | null>(null);
  const [requests, setRequests] = React.useState<Request[]>([]);
  const [binName, setBinName] = React.useState('');
  const [loading, setLoading] = React.useState(false);
  const [copied, setCopied] = React.useState<string | null>(null);

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

  const createBin = async (e: React.FormEvent) => {
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

  const loadRequests = async (binId: string) => {
    try {
      const res = await fetch(`/api/bins/${binId}/requests`);
      const data = await res.json();
      setRequests(data);
      setBins(bins.map(bin => 
        bin.id === binId ? { ...bin, requestCount: data.length } : bin
      ));
    } catch (err) {
      console.error('Error loading requests:', err);
    }
  };

  const selectBin = async (binId: string) => {
    setSelectedBin(binId);
    loadRequests(binId);
  };

  const deleteBin = async (binId: string) => {
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

  const copyToClipboard = (text: string) => {
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
        <h1>Request Bin - v3</h1>
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
                    {bin.requestCount !== undefined && (
                      <span className="request-count">{bin.requestCount} requests</span>
                    )}
                    <button 
                      className="delete-btn"
                      onClick={(e: React.MouseEvent) => { e.stopPropagation(); deleteBin(bin.id); }}
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
