import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { MutationError } from "./MutationError";

describe("MutationError", () => {
  it("renders nothing when there is no error", () => {
    const { container } = render(<MutationError isError={false} error={null} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders the error message with an alert role when isError is true", () => {
    render(<MutationError isError={true} error={new Error("boom")} />);
    expect(screen.getByRole("alert")).toHaveTextContent("boom");
  });

  it("renders nothing if isError is true but error is null (defensive)", () => {
    const { container } = render(<MutationError isError={true} error={null} />);
    expect(container).toBeEmptyDOMElement();
  });
});
