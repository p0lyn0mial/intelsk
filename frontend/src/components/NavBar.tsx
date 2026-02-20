import { useState, useEffect } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { getSettings } from '../api/client';

export default function NavBar() {
  const { t, i18n } = useTranslation();
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);

  const { data: settingsData } = useQuery({
    queryKey: ['settings'],
    queryFn: getSettings,
  });
  const systemName = (settingsData?.settings['general.system_name'] as string) || t('nav.title');

  useEffect(() => {
    document.title = systemName;
  }, [systemName]);

  const toggleLang = () => {
    const next = i18n.language === 'en' ? 'pl' : 'en';
    i18n.changeLanguage(next);
    localStorage.setItem('lang', next);
  };

  const navLinks = [
    { to: '/', label: t('nav.title') },
    { to: '/cameras', label: t('nav.cameras') },
    { to: '/settings', label: t('nav.settings') },
  ];

  return (
    <nav className="bg-gray-900 text-white">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex items-center justify-between h-14">
          <div className="flex items-center gap-6">
            <Link to="/" className="text-lg font-bold tracking-tight">
              {systemName}
            </Link>
            <div className="hidden sm:flex gap-4">
              {navLinks.slice(1).map((link) => (
                <Link
                  key={link.to}
                  to={link.to}
                  className={`px-3 py-1.5 rounded text-sm ${
                    location.pathname === link.to
                      ? 'bg-gray-700'
                      : 'hover:bg-gray-800'
                  }`}
                >
                  {link.label}
                </Link>
              ))}
            </div>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={toggleLang}
              className="px-2 py-1 text-xs font-mono border border-gray-600 rounded hover:bg-gray-800"
            >
              {i18n.language === 'en' ? 'PL' : 'EN'}
            </button>

            {/* Hamburger */}
            <button
              className="sm:hidden p-2"
              onClick={() => setMenuOpen(!menuOpen)}
              aria-label="Menu"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                {menuOpen ? (
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                ) : (
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
                )}
              </svg>
            </button>
          </div>
        </div>
      </div>

      {/* Mobile menu */}
      {menuOpen && (
        <div className="sm:hidden border-t border-gray-700 px-4 pb-3 pt-2 space-y-1">
          {navLinks.map((link) => (
            <Link
              key={link.to}
              to={link.to}
              onClick={() => setMenuOpen(false)}
              className={`block px-3 py-2 rounded text-sm ${
                location.pathname === link.to
                  ? 'bg-gray-700'
                  : 'hover:bg-gray-800'
              }`}
            >
              {link.label}
            </Link>
          ))}
        </div>
      )}
    </nav>
  );
}
