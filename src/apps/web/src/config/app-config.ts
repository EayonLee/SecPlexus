import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "SecPlexus",
  url: process.env.NEXT_PUBLIC_APP_URL ?? "http://localhost:3000",
  repository: "https://github.com/EayonLee/SecPlexus",
  version: packageJson.version,
  copyright: `© ${currentYear}, SecPlexus.`,
};
