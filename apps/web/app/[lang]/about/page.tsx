import * as React from "react";
import { coerceLang, isLang } from "@/lib/i18n";
import { notFound } from "next/navigation";
import type { Metadata } from "next";

type PageParams = { lang: string };

export async function generateMetadata({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<Metadata> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) return {};
  const lang = coerceLang(rawLang);
  return {
    title: lang === "es" ? "Sobre nosotros" : "About",
  };
}

export default async function AboutPage({
  params,
}: {
  params: Promise<PageParams>;
}): Promise<React.ReactElement> {
  const { lang: rawLang } = await params;
  if (!isLang(rawLang)) notFound();
  const lang = coerceLang(rawLang);
  return lang === "es" ? <AboutEs /> : <AboutEn />;
}

function AboutEs(): React.ReactElement {
  return (
    <article className="mx-auto max-w-3xl px-6 py-16 text-learn-warm space-y-8">
      <header>
        <p className="learn-eyebrow">Sobre nosotros</p>
        <h1 className="mt-3 text-4xl leading-tight text-learn-warmHi">
          Ensenamos Claude Code usandolo.
        </h1>
      </header>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">Quien esta detras</h2>
        <p>
          Platform Engineering es una consultora fundada por Mikel Martin. Trabajamos
          AI-native desde dentro. Nuestra linea IA nace porque varios clientes
          nos pidieron primero formacion en Claude Code antes que DevOps.
          learn.example.com es como respondemos a esa demanda.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">Que es Claude Code</h2>
        <p>
          Claude Code es el agente de Anthropic que programa, escribe,
          investiga y automatiza desde una terminal. No es una GUI ni un copilot
          limitado a autocompletar. Puedes pedirle cosas grandes, y el se
          encarga de leer, ejecutar comandos, escribir ficheros y volver con
          resultados.
        </p>
        <p>
          Nuestra tesis es simple. La gente que nunca abrio una terminal puede
          volverse productiva con Claude Code en dias, no en anos. Lo que les
          frena no es la tecnologia, es la ausencia de una primera hora guiada.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">Tu sandbox es solo tuya</h2>
        <p>
          Cada leccion abre una maquina virtual fresca que vive solo mientras tu
          estas conectado. Nadie mas la ve. Cuando cierras, se reap. No tocamos
          tu ordenador ni tus datos personales. No necesitas NDA para empezar,
          porque no te pedimos informacion confidencial.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">Que hacemos con tus datos</h2>
        <p>
          Guardamos tu email para mandar el enlace de entrada, tu progreso para
          que no repitas lecciones, y los eventos de tu sandbox (que tool
          ejecuto Claude, que archivo escribio, el resultado del paso) para que
          puedas volver y retomar. Nada mas. No vendemos datos. No entrenamos
          modelos con tu contenido.
        </p>
        <p>
          Puedes exportar o borrar tu cuenta desde ajustes en cualquier momento.
          La infraestructura vive en un host propio en Mexico City, detras de
          Cloudflare. El token de Claude lo gestiona nuestra instancia.
        </p>
      </section>
    </article>
  );
}

function AboutEn(): React.ReactElement {
  return (
    <article className="mx-auto max-w-3xl px-6 py-16 text-learn-warm space-y-8">
      <header>
        <p className="learn-eyebrow">About</p>
        <h1 className="mt-3 text-4xl leading-tight text-learn-warmHi">
          We teach Claude Code by using it.
        </h1>
      </header>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">Who is behind this</h2>
        <p>
          Platform Engineering is a consultancy founded by Mikel Martin. We work
          AI-native from the inside out. Our AI line exists because several
          clients asked for Claude Code training before they asked for DevOps.
          learn.example.com is how we answer that.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">What Claude Code is</h2>
        <p>
          Claude Code is Anthropic's agent that codes, writes, researches and
          automates from a terminal. It is not a GUI, and it is not an
          autocomplete copilot. You can ask it for big things, and it reads
          files, runs commands, writes files, and comes back with results.
        </p>
        <p>
          Our thesis is simple. People who never opened a terminal can become
          productive with Claude Code in days, not years. What blocks them is
          not the technology, it is the missing guided first hour.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">Your sandbox is only yours</h2>
        <p>
          Every lesson boots a fresh virtual machine that lives only while you
          are connected. Nobody else sees it. When you close, it is reaped. We
          never touch your computer or your personal data. You do not need an
          NDA to start, because we do not ask for confidential information.
        </p>
      </section>

      <section className="space-y-4">
        <h2 className="text-2xl text-learn-warmHi">What we do with your data</h2>
        <p>
          We keep your email to send the sign-in link, your progress so lessons
          do not repeat, and your sandbox events (which tool Claude used, what
          file it wrote, the result of the step) so you can come back. Nothing
          else. We do not sell data. We do not train models on your content.
        </p>
        <p>
          You can export or delete your account from settings at any time. The
          infrastructure lives on our own host in Mexico City, behind
          Cloudflare. The Claude token is managed by our instance.
        </p>
      </section>
    </article>
  );
}
