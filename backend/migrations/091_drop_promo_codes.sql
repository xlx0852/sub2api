-- Remove registration promo code feature
DROP TABLE IF EXISTS promo_code_usages;
DROP TABLE IF EXISTS promo_codes;
DELETE FROM settings WHERE key = 'promo_code_enabled';
