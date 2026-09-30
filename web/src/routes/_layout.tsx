import { A, Outlet, useLocation } from "@solidjs/router";

export default function AppLayout() {
  const location = useLocation();
  return (
    <div class="flex min-h-screen">
      <nav class="w-52 bg-sidebar-bg border-r border-border p-4 fixed h-screen">
        <div class="mb-6">
          <A href="/" class="text-lg font-bold text-accent no-underline">caretaker</A>
        </div>
        <div class="flex flex-col gap-1">
          <A
            href="/"
            class="text-text-dim no-underline px-3 py-2 rounded-md text-sm hover:bg-border hover:text-text transition-colors"
            classList={{ "bg-accent text-white!": location.pathname === "/" }}
          >
            Overview
          </A>
          <A
            href="/history"
            class="text-text-dim no-underline px-3 py-2 rounded-md text-sm hover:bg-border hover:text-text transition-colors"
            classList={{ "bg-accent text-white!": location.pathname === "/history" }}
          >
            Deployment History
          </A>
        </div>
      </nav>
      <main class="flex-1 ml-52 p-8 max-w-6xl">
        <Outlet />
      </main>
    </div>
  );
}
