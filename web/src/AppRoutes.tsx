import { Navigate, Route, Routes } from "react-router-dom";
import { ChannelScheduleScreen } from "./screens/ChannelScheduleScreen";
import { ChannelsListScreen } from "./screens/ChannelsListScreen";
import { GuideScreen } from "./screens/GuideScreen";
import { LibraryScreen } from "./screens/LibraryScreen";
import { SettingsScreen } from "./screens/SettingsScreen";
import { TVIndexScreen } from "./screens/TVIndexScreen";
import { TVScreen } from "./screens/TVScreen";

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/guide" replace />} />
      <Route path="/guide" element={<GuideScreen />} />
      <Route path="/tv" element={<TVIndexScreen />} />
      <Route path="/tv/:channelId" element={<TVScreen />} />
      <Route path="/library" element={<LibraryScreen />} />
      <Route path="/channels" element={<ChannelsListScreen />} />
      <Route path="/channels/:id" element={<ChannelScheduleScreen />} />
      <Route path="/settings" element={<SettingsScreen />} />
    </Routes>
  );
}
