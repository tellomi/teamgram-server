-- Email OTP accounts.
-- users.email is unique and nullable (phone accounts keep NULL).
-- users.phone becomes nullable so email-only accounts store NULL, not ''.
-- InnoDB unique indexes allow many NULLs, so existing phone rows stay unique
-- and many email-only rows can share a NULL phone.
-- Account delete sets email = NULL (see UsersDAO.Delete) so the address can
-- be registered again.

CALL teamgram_add_column('users', 'email', 'ALTER TABLE `users` ADD `email` varchar(255) COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL AFTER `phone`');
CALL teamgram_add_index('users', 'email', 'ALTER TABLE `users` ADD UNIQUE KEY `email` (`email`)');
CALL teamgram_change_column('users', 'phone', 'ALTER TABLE `users` MODIFY `phone` varchar(32) COLLATE utf8mb4_unicode_ci NULL DEFAULT NULL');
