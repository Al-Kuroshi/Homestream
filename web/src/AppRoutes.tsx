import { Navigate, Route, Routes } from "react-router-dom";
import { ChannelsListScreen } from "./screens/ChannelsListScreen";
import { LibraryScreen } from "./screens/LibraryScreen";
import { SettingsScreen } from "./screens/SettingsScreen";
import { TVIndexScreen } from "./screens/TVIndexScreen";
import { TVScreen } from "./screens/TVScreen";

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/tv" replace />} />
      <Route path="/tv" element={<TVIndexScreen />} />
      <Route path="/tv/:channelId" element={<TVScreen />} />
      <Route path="/library" element={<LibraryScreen />} />
      <Route path="/channels" element={<ChannelsListScreen />} />
      <Route path="/settings" element={<SettingsScreen />} />
    </Routes>
  );
}
