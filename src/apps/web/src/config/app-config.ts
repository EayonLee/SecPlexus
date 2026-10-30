import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "SecPlexus",
  repository: "https://github.com/EayonLee/SecPlexus",
  version: packageJson.version,
  copyright: `© ${currentYear}, SecPlexus.`,
};
