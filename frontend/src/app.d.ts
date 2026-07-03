declare global {
	namespace App {
		interface Locals {
			token?: string;
			user?: {
				user_id: string;
				email: string;
				is_admin: boolean;
			};
		}
	}
}

export {};
