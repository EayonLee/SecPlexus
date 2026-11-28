import { GeistMono } from "geist/font/mono";
import { GeistPixelSquare } from "geist/font/pixel";
import { GeistSans } from "geist/font/sans";

const fontRegistry = {
  montserrat: "Montserrat",
  geist: "Geist",
  inter: "Inter",
  notoSans: "Noto Sans",
  nunitoSans: "Nunito Sans",
  figtree: "Figtree",
  roboto: "Roboto",
  raleway: "Raleway",
  dmSans: "DM Sans",
  publicSans: "Public Sans",
  outfit: "Outfit",
  geistMono: "Geist Mono",
  geistPixelSquare: "Geist Pixel Square",
  jetBrainsMono: "JetBrains Mono",
  notoSerif: "Noto Serif",
  robotoSlab: "Roboto Slab",
  merriweather: "Merriweather",
  lora: "Lora",
  playfairDisplay: "Playfair Display",
} as const;

export type FontKey = keyof typeof fontRegistry;

export const fontKeys = Object.keys(fontRegistry) as FontKey[];

export const fontVars = [GeistSans.variable, GeistMono.variable, GeistPixelSquare.variable].join(" ");

export const fontOptions = fontKeys.map((key) => ({ key, label: fontRegistry[key] }));
