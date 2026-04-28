import { Link, useLocation } from 'react-router-dom';
import { ReactNode } from 'react';
import { 
  ShieldCheckIcon, 
  ServerIcon,
  ServerStackIcon,
  UserGroupIcon,
  ChartBarIcon,
  MagnifyingGlassIcon,
  ExclamationTriangleIcon,
  CodeBracketIcon,
  BookOpenIcon
} from '@heroicons/react/24/outline';
import { 
  ServerIcon as ServerIconSolid,
  ServerStackIcon as ServerStackIconSolid,
  UserGroupIcon as UserGroupIconSolid,
  ChartBarIcon as ChartBarIconSolid,
  MagnifyingGlassIcon as MagnifyingGlassIconSolid,
  ExclamationTriangleIcon as ExclamationTriangleIconSolid,
  CodeBracketIcon as CodeBracketIconSolid,
  BookOpenIcon as BookOpenIconSolid
} from '@heroicons/react/24/solid';

interface LayoutProps {
  children: ReactNode;
}

export default function Layout({ children }: LayoutProps) {
  const location = useLocation();

  const isActive = (path: string) => location.pathname === path || location.pathname.startsWith(path + '/');

  const navItems = [
    { path: '/', label: 'Risk Dashboard', icon: ChartBarIcon, iconSolid: ChartBarIconSolid },
    { path: '/lenses', label: 'Lenses', icon: UserGroupIcon, iconSolid: UserGroupIconSolid },
    { path: '/executive', label: 'Executive Summary', icon: ChartBarIcon, iconSolid: ChartBarIconSolid },
    { path: '/setup', label: 'Setup', icon: BookOpenIcon, iconSolid: BookOpenIconSolid },
    { path: '/connect', label: 'Connect Systems', icon: ServerStackIcon, iconSolid: ServerStackIconSolid },
    { path: '/connectors', label: 'Connector Health', icon: ServerIcon, iconSolid: ServerIconSolid },
    { path: '/iql', label: 'IQL Search', icon: MagnifyingGlassIcon, iconSolid: MagnifyingGlassIconSolid },
    { path: '/toxic-combo', label: 'Toxic Combo', icon: ExclamationTriangleIcon, iconSolid: ExclamationTriangleIconSolid },
    { path: '/custom-rules', label: 'Custom Rules', icon: CodeBracketIcon, iconSolid: CodeBracketIconSolid },
  ];

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_top,_#0f172a,_#020617_55%,_#000000)] text-slate-100">
      <nav className="sticky top-0 z-50 border-b border-cyan-500/30 bg-slate-950/80 backdrop-blur-xl">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex justify-between h-16">
            <div className="flex items-center space-x-6">
              <div className="flex items-center space-x-3">
                <div className="relative h-9 w-9 rounded-xl bg-gradient-to-br from-cyan-400 via-cyan-500 to-emerald-400 flex items-center justify-center shadow-[0_0_25px_rgba(34,211,238,0.6)]">
                  <ShieldCheckIcon className="h-5 w-5 text-slate-950" />
                  <span className="pointer-events-none absolute inset-0 rounded-xl border border-white/10" />
                </div>
                <div>
                  <div className="text-xs uppercase tracking-[0.25em] text-cyan-300/80">
                    Identity Telemetry
                  </div>
                  <h1 className="text-lg font-semibold leading-tight">
                    <span className="bg-gradient-to-r from-slate-50 via-cyan-200 to-emerald-300 bg-clip-text text-transparent">
                      Flight Recorder
                    </span>
                  </h1>
                </div>
              </div>
              <div className="hidden md:flex items-center space-x-1">
                {navItems.map((item) => {
                  const active = isActive(item.path);
                  const Icon = active ? item.iconSolid : item.icon;
                  return (
                    <Link
                      key={item.path}
                      to={item.path}
                      className={`relative inline-flex items-center px-3.5 py-1.5 rounded-full text-xs font-medium tracking-wide transition-all duration-200 ${
                        active
                          ? 'bg-cyan-500/15 text-cyan-200 shadow-[0_0_0_1px_rgba(34,211,238,0.4)]'
                          : 'text-slate-300/80 hover:text-cyan-200 hover:bg-slate-800/60'
                      }`}
                    >
                      <Icon
                        className={`h-4 w-4 mr-2 ${
                          active ? 'text-cyan-300' : 'text-slate-400 group-hover:text-cyan-300'
                        }`}
                      />
                      {item.label}
                    </Link>
                  );
                })}
              </div>
            </div>
          </div>
        </div>
      </nav>
      <main className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
        {children}
      </main>
    </div>
  );
}
