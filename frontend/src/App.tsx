import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import IdentityProfile from './pages/IdentityProfile';
import ExplainabilityView from './pages/ExplainabilityView';
import ConnectorHealth from './pages/ConnectorHealth';
import ConnectSystemsPage from './pages/ConnectSystemsPage';
import LensesPage from './pages/LensesPage';
import IQLSearchPage from './pages/IQLSearchPage';
import ToxicComboPage from './pages/ToxicComboPage';
import ExecutiveDashboard from './pages/ExecutiveDashboard';
import CustomRulesPage from './pages/CustomRulesPage';
import SetupPage from './pages/SetupPage';

function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/setup" element={<SetupPage />} />
          <Route path="/identities/:id" element={<IdentityProfile />} />
          <Route path="/identities/:id/explain" element={<ExplainabilityView />} />
          <Route path="/connectors" element={<ConnectorHealth />} />
          <Route path="/connect" element={<ConnectSystemsPage />} />
          <Route path="/lenses" element={<LensesPage />} />
          <Route path="/iql" element={<IQLSearchPage />} />
          <Route path="/toxic-combo" element={<ToxicComboPage />} />
          <Route path="/executive" element={<ExecutiveDashboard />} />
          <Route path="/custom-rules" element={<CustomRulesPage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  );
}

export default App;
