-- 初始种子数据

-- 插入题目
INSERT INTO problems (id, title, difficulty, description, input_format, output_format, constraints, created_by) VALUES
(1, '两数之和', 'Easy', '给定一个整数数组和一个目标和，返回两个数字的索引，使它们相加等于目标值。', '第一行：用空格分隔的整数数组\n第二行：目标整数', '两个用空格分隔的整数，表示索引（从0开始）', '• 2 ≤ 数组长度 ≤ 10⁴\n• -10⁹ ≤ array[i] ≤ 10⁹\n• 只存在一个有效答案', 1),
(2, '反转字符串', 'Easy', '编写一个函数来反转字符串。输入字符串以字符数组的形式给出。', '一行包含一个字符串', '反转后的字符串', '• 1 ≤ 字符串长度 ≤ 10⁵\n• 字符串由可打印的ASCII字符组成', 1),
(3, '有效的回文串', 'Easy', '给定一个字符串，判断它是否是回文串，只考虑字母和数字字符，并忽略大小写。', '一行包含一个字符串', '如果是回文串返回 true，否则返回 false', '• 1 ≤ 字符串长度 ≤ 2 × 10⁵\n• 字符串由可打印的ASCII字符组成', 1),
(4, '最大子数组', 'Medium', '给定一个整数数组，找到具有最大和的连续子数组并返回其和。', '用空格分隔的整数，表示数组', '一个整数，表示最大和', '• 1 ≤ 数组长度 ≤ 10⁵\n• -10⁴ ≤ array[i] ≤ 10⁴', 1),
(5, '合并两个有序数组', 'Medium', '给定两个有序整数数组，将它们合并成一个有序数组。', '第一行：第一个有序数组（用空格分隔的整数）\n第二行：第二个有序数组（用空格分隔的整数）', '用空格分隔的整数，表示合并后的有序数组', '• 0 ≤ 数组长度 ≤ 1000\n• -10⁶ ≤ array[i] ≤ 10⁶', 1),
(6, '二分查找', 'Easy', '给定一个有序数组和一个目标值，如果找到目标值则返回其索引，否则返回 -1。', '第一行：有序数组（用空格分隔的整数）\n第二行：目标整数', '目标值的索引（从0开始）或 -1（如果未找到）', '• 1 ≤ 数组长度 ≤ 10⁴\n• -10⁴ ≤ array[i], target ≤ 10⁴\n• 所有元素都是唯一的', 1)
ON CONFLICT (id) DO UPDATE SET
  title = EXCLUDED.title,
  difficulty = EXCLUDED.difficulty,
  description = EXCLUDED.description;

-- 插入题目标签
INSERT INTO problem_tags (problem_id, tag) VALUES
(1, '数组'), (1, '哈希表'),
(2, '字符串'), (2, '双指针'),
(3, '字符串'), (3, '双指针'),
(4, '数组'), (4, '动态规划'),
(5, '数组'), (5, '双指针'),
(6, '数组'), (6, '二分查找')
ON CONFLICT (problem_id, tag) DO NOTHING;

-- 插入题目示例
INSERT INTO problem_examples (problem_id, input, output, explanation, display_order) VALUES
-- 两数之和
(1, '2 7 11 15\n9', '0 1', 'nums[0] + nums[1] = 2 + 7 = 9', 0),
(1, '3 2 4\n6', '1 2', 'nums[1] + nums[2] = 2 + 4 = 6', 1),
-- 反转字符串
(2, 'hello', 'olleh', '字符串 "hello" 反转后是 "olleh"', 0),
(2, 'world', 'dlrow', NULL, 1),
-- 有效的回文串
(3, 'A man, a plan, a canal: Panama', 'true', '移除非字母数字字符并忽略大小写后："amanaplanacanalpanama" 是回文串', 0),
(3, 'race a car', 'false', NULL, 1),
-- 最大子数组
(4, '-2 1 -3 4 -1 2 1 -5 4', '6', '子数组 [4,-1,2,1] 的和最大 = 6', 0),
(4, '1', '1', NULL, 1),
-- 合并两个有序数组
(5, '1 3 5\n2 4 6', '1 2 3 4 5 6', NULL, 0),
(5, '1\n2', '1 2', NULL, 1),
-- 二分查找
(6, '1 2 3 4 5\n3', '2', NULL, 0),
(6, '1 2 3 4 5\n6', '-1', NULL, 1);

-- 插入测试用例
INSERT INTO test_cases (problem_id, input, expected_output, is_sample, display_order) VALUES
-- 两数之和
(1, '2 7 11 15\n9', '0 1', true, 0),
(1, '3 2 4\n6', '1 2', true, 1),
(1, '3 3\n6', '0 1', false, 2),
-- 反转字符串
(2, 'hello', 'olleh', true, 0),
(2, 'world', 'dlrow', true, 1),
(2, 'a', 'a', false, 2),
-- 有效的回文串
(3, 'A man, a plan, a canal: Panama', 'true', true, 0),
(3, 'race a car', 'false', true, 1),
(3, 'a', 'true', false, 2),
-- 最大子数组
(4, '-2 1 -3 4 -1 2 1 -5 4', '6', true, 0),
(4, '1', '1', true, 1),
(4, '5 4 -1 7 8', '23', false, 2),
-- 合并两个有序数组
(5, '1 3 5\n2 4 6', '1 2 3 4 5 6', true, 0),
(5, '1\n2', '1 2', true, 1),
(5, '1 2 3\n4 5 6', '1 2 3 4 5 6', false, 2),
-- 二分查找
(6, '1 2 3 4 5\n3', '2', true, 0),
(6, '1 2 3 4 5\n6', '-1', true, 1),
(6, '1\n1', '0', false, 2);

-- 更新序列
SELECT setval('problems_id_seq', (SELECT MAX(id) FROM problems));
