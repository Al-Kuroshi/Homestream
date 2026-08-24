import { NavLink } from "react-router-dom";
import "./Sidebar.css";

const NAV_ITEMS = [
  { to: "/tv", label: "TV" },
  { to: "/guide", label: "Guide" },
  { to: "/library", label: "Library" },
  { to: "/channels", label: "Channels" },
  { to: "/settings", label: "Settings" },
] as const;

export function Sidebar() {
  return (
    <nav className="sidebar" aria-label="Main navigation">
      <div className="sidebar-title">Personal TV</div>
      <ul>
        {NAV_ITEMS.map((item) => (
          <li key={item.to}>
            <NavLink to={item.to} className={({ isActive }) => (isActive ? "active" : undefined)}>
              {item.label}
            </NavLink>
          </li>
        ))}
      </ul>
    </nav>
  );
}
