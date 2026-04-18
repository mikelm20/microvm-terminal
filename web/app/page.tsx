import AuthGate from "@/components/AuthGate";
import Landing from "@/components/Landing";

export default function HomePage() {
  return (
    <AuthGate>
      <Landing />
    </AuthGate>
  );
}
