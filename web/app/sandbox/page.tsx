import AuthGate from "@/components/AuthGate";
import SandboxClient from "@/components/SandboxClient";

export default function SandboxPage() {
  return (
    <AuthGate>
      <SandboxClient />
    </AuthGate>
  );
}
