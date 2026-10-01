import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function fmt(n: number) {
  return n.toLocaleString("en-US");
}

export function randHex(len = 8) {
  let s = "";
  while (s.length < len) s += Math.random().toString(16).slice(2);
  return s.slice(0, len);
}
