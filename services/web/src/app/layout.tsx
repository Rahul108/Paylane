import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Paylane — Microservice Payment Platform',
  description: 'Local-only multi-service payment engine with mock PGWs and JWE mesh',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
