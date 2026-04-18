import Landing from "@/components/Landing";

// The landing is public. It's a zero-backend static page so it can live on
// a plain VPS without the control-plane Go server. Auth still wraps /sandbox
// and /lessons/* (those routes carry their own AuthGate) because those are
// the cost-bearing routes that need the control plane.
export default function HomePage() {
  return <Landing />;
}
