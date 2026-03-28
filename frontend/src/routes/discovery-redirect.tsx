import { useEffect } from "react";
import { useNavigate } from "@tanstack/react-router";

export function DiscoveryRedirect() {
  const navigate = useNavigate();
  useEffect(() => {
    void navigate({ to: "/networks" });
  }, [navigate]);
  return null;
}
