// Small synthetic TypeScript file for unit tests.

export interface User {
  id: number;
  name: string;
  email: string;
}

export type UserId = number;

export function greetUser(user: User): string {
  return `Hello, ${user.name}!`;
}

export function getUserById(id: UserId): User | undefined {
  return undefined;
}
