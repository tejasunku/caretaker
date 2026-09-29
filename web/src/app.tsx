// @refresh reload
import { MetaProvider } from "@solidjs/meta";
import { A, Outlet, useLocation } from "@solidjs/router";
import "./app.css";

export default function App() {
  const location = useLocation();
  return (
    <MetaProvider>
      <div class="app">
        <nav class="sidebar">
          <div class="sidebar-header">
            <A href="/" class="logo">caretaker</A>
          </div>
          <div class="nav-links">
            <A href="/" class:list={{ active: location.pathname === "/" }}>
              Overview
            </A>
          </div>
        </nav>
        <main class="content">
          <Outlet />
        </main>
      </div>
    </MetaProvider>
  );
}
