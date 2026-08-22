import { useMemo, useState } from "react";
import { useMediaItems } from "../api/media";
import { useSources } from "../api/sources";
import "./LibraryScreen.css";

export function LibraryScreen() {
  const { data: items, isLoading, isError } = useMediaItems();
  const { data: sources } = useSources();
  const [search, setSearch] = useState("");
  const [sourceFilter, setSourceFilter] = useState<number | "all">("all");
  const [invalidOnly, setInvalidOnly] = useState(false);

  const sourceNameById = useMemo(() => {
    const map = new Map<number, string>();
    for (const s of sources ?? []) map.set(s.id, s.name);
    return map;
  }, [sources]);

  const filtered = useMemo(() => {
    return (items ?? []).filter((item) => {
      if (search && !item.title.toLowerCase().includes(search.toLowerCase())) return false;
      if (sourceFilter !== "all" && item.source_id !== sourceFilter) return false;
      if (invalidOnly && !item.invalid) return false;
      return true;
    });
  }, [items, search, sourceFilter, invalidOnly]);

  if (isLoading) return <p>Loading media…</p>;
  if (isError) return <p role="alert">Failed to load media library.</p>;

  return (
    <section>
      <h1>Library</h1>
      <div className="library-filters">
        <input
          type="search"
          placeholder="Search titles…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          aria-label="Search titles"
        />
        <select
          value={sourceFilter}
          onChange={(e) => setSourceFilter(e.target.value === "all" ? "all" : Number(e.target.value))}
          aria-label="Filter by source"
        >
          <option value="all">All sources</option>
          {(sources ?? []).map((s) => (
            <option key={s.id} value={s.id}>{s.name}</option>
          ))}
        </select>
        <label>
          <input type="checkbox" checked={invalidOnly} onChange={(e) => setInvalidOnly(e.target.checked)} />
          Invalid only
        </label>
      </div>
      <table>
        <thead>
          <tr>
            <th>Title</th><th>Duration</th><th>Source</th><th>Codec</th><th>Container</th><th>Status</th>
          </tr>
        </thead>
        <tbody>
          {filtered.map((item) => (
            <tr key={item.id}>
              <td>{item.title}</td>
              <td>{formatDuration(item.duration_sec)}</td>
              <td>{sourceNameById.get(item.source_id) ?? item.source_id}</td>
              <td>{item.video_codec || "—"}/{item.audio_codec || "—"}</td>
              <td>{item.container || "—"}</td>
              <td>{item.invalid ? "Invalid" : "OK"}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {filtered.length === 0 && <p>No media matches the current filters.</p>}
    </section>
  );
}

function formatDuration(seconds: number): string {
  const total = Math.round(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return h > 0
    ? `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
    : `${m}:${String(s).padStart(2, "0")}`;
}
