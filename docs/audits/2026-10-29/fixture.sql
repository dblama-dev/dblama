-- Run only in a disposable, empty test database.
SET NAMES utf8mb4;
CREATE TABLE customers (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL, email VARCHAR(255) NOT NULL DEFAULT '', status ENUM('active','inactive') NOT NULL DEFAULT 'active', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, notes TEXT NULL);
INSERT INTO customers(name,email,notes) VALUES ('Анна','anna@example.test','Короткая заметка'),('Борис','boris@example.test',REPEAT('Длинное описание клиента для проверки отображения и ширины таблицы. ',15));
INSERT INTO customers(name,email,notes) SELECT CONCAT('Клиент ', n), CONCAT('customer',n,'@example.test'), NULL FROM JSON_TABLE('[3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60]', '$[*]' COLUMNS (n INT PATH '$')) j;
CREATE TABLE orders (id INT AUTO_INCREMENT PRIMARY KEY, customer_id INT NOT NULL, amount DECIMAL(12,2) NOT NULL DEFAULT 0, state VARCHAR(30) NOT NULL DEFAULT 'new', FOREIGN KEY(customer_id) REFERENCES customers(id));
INSERT INTO orders(customer_id,amount) VALUES (1,123.45),(2,900.00);
CREATE TABLE empty_values (id INT AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL, notes TEXT NULL);
INSERT INTO empty_values(name,notes) VALUES ('','');
CREATE VIEW active_customers AS SELECT id,name FROM customers WHERE status='active';
