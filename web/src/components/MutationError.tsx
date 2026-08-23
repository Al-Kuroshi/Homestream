import "./MutationError.css";

interface Props {
  isError: boolean;
  error: Error | null;
}

// Shared inline error display for a failed mutation (spec §6: failed
// add/remove/rescan/create/delete actions show an error near the action
// that failed). Renders nothing once the mutation is retried and succeeds,
// since TanStack Query resets mutation state on the next `.mutate()` call.
export function MutationError({ isError, error }: Props) {
  if (!isError || !error) return null;
  return (
    <p role="alert" className="mutation-error">
      {error.message}
    </p>
  );
}
