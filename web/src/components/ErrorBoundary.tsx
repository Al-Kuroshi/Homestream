import { Component, type ReactNode } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

// The route-level last-resort safety net (spec §6): an unexpected render
// error anywhere under this boundary shows a fallback message instead of
// blanking the whole app. This is not the primary error-handling path —
// TanStack Query's isError/error states (handled per-screen) are — this
// only catches what those can't (a bug in render logic itself).
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  render() {
    if (this.state.error) {
      return <p role="alert">Something went wrong: {this.state.error.message}</p>;
    }
    return this.props.children;
  }
}
