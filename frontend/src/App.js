import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';

function App() {
  const [apiStatus, setApiStatus] = React.useState(null);
  const [dbTime, setDbTime] = React.useState(null);
  const [nginxHealth, setNginxHealth] = React.useState(null);

  React.useEffect(() => {
    fetch('/api/v1')
      .then(res => res.json())
      .then(data => setApiStatus(data))
      .catch(err => setApiStatus({ error: err.message }));

    fetch('/db/time')
      .then(res => res.json())
      .then(data => setDbTime(data.db_time))
      .catch(err => setDbTime(err.message));

    fetch('/health')
      .then(res => res.text())
      .then(data => setNginxHealth(data.trim()))
      .catch(err => setNginxHealth(err.message));
  }, []);

  return (
    <div className="app">
      <header>
        <h1>Platform Engineering</h1>
      </header>
      <main>
        <section>
          <h2>Welcome to Your Application</h2>
          <p>This is a production-ready full-stack application.</p>
        </section>
        <section>
          <h3>Nginx Health</h3>
          {nginxHealth ? (
            <pre>{JSON.stringify(nginxHealth, null, 2)}</pre>
          ) : (
            <p>Loading...</p>
          )}
        </section>
        <section>
          <h3>API Status</h3>
          {apiStatus ? (
            <pre>{JSON.stringify(apiStatus, null, 2)}</pre>
          ) : (
            <p>Loading...</p>
          )}
        </section>
        <section>
          <h3>Database Time</h3>
          {dbTime ? (
            <pre>{JSON.stringify(dbTime, null, 2)}</pre>
          ) : (
            <p>Loading...</p>
          )}
        </section>
      </main>
    </div>
  );
}

export default App;
