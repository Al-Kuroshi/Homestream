import { useState, type FormEvent } from "react";
import { useMediaItems } from "../api/media";
import { useCreateSource, useDeleteSource, useScanSource, useSources } from "../api/sources";
import "./SettingsScreen.css";

export function SettingsScreen() {
  const { data: sources, isLoading, isError } = useSources();
  const { data: media } = useMediaItems();
  const createSource = useCreateSource();
  const deleteSource = useDeleteSource();
  const scanSource = useScanSource();
  const [name, setName] = useState("");
  const [path, setPath] = useState("");
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<number | null>(null);

  function itemCount(sourceId: number): number {
    return (media ?? []).filter((m) => m.source_id === sourceId).length;
  }

  function handleAdd(e: FormEvent) {
    e.preventDefault();
    createSource.mutate(
      { name, path },
      { onSuccess: () => { setName(""); setPath(""); } }
    );
  }

  if (isLoading) return <p>Loading sources…</p>;
  if (isError) return <p role="alert">Failed to load media sources.</p>;

  return (
    <section>
      <h1>Settings</h1>
      <h2>Media Sources</h2>
      <ul className="source-list">
        {(sources ?? []).map((source) => (
          <li key={source.id}>
            <div>
              <strong>{source.name}</strong>
              <div className="source-path">{source.path} — {itemCount(source.id)} item(s)</div>
            </div>
            <div className="source-actions">
              <button
                onClick={() => scanSource.mutate(source.id)}
                disabled={scanSource.isPending && scanSource.variables === source.id}
              >
                {scanSource.isPending && scanSource.variables === source.id ? "Scanning…" : "Rescan"}
              </button>
              {confirmingDeleteId === source.id ? (
                <>
                  <span>Remove this source and all its media/programs?</span>
                  <button
                    onClick={() =>
                      deleteSource.mutate(source.id, { onSettled: () => setConfirmingDeleteId(null) })
                    }
                  >
                    Confirm remove
                  </button>
                  <button onClick={() => setConfirmingDeleteId(null)}>Cancel</button>
                </>
              ) : (
                <button onClick={() => setConfirmingDeleteId(source.id)}>Remove</button>
              )}
            </div>
          </li>
        ))}
      </ul>
      {(sources ?? []).length === 0 && <p>No media sources configured yet.</p>}

      <h2>Add a source</h2>
      <form onSubmit={handleAdd}>
        <label>
          Name
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          Path
          <input value={path} onChange={(e) => setPath(e.target.value)} required />
        </label>
        <button type="submit" disabled={createSource.isPending}>Add source</button>
        {createSource.isError && <p role="alert">{createSource.error.message}</p>}
      </form>
    </section>
  );
}
